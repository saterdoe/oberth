package api

import (
	stdcontext "context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/cost"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/internal/mcp"
	"github.com/saterdoe/oberth/internal/nativepicker"
	"github.com/saterdoe/oberth/internal/permission"
	"github.com/saterdoe/oberth/internal/structuredoutput"
	"github.com/saterdoe/oberth/internal/vault"
)

type Server struct {
	pool          *pgxpool.Pool
	providers     *repos.ProviderRepo
	routingRules  *repos.RoutingRuleRepo
	sessions      *repos.SessionRepo
	tasks         *repos.TaskRepo
	costLogs      *repos.CostLogRepo
	budgets       *repos.BudgetRepo
	audit         *repos.AuditRepo
	execution     *repos.ExecutionLogRepo
	approvalGates *repos.ApprovalGateRepo
	costTracker   *cost.Tracker
	searcher      *context.Searcher
	indexer       *context.Indexer
	router        *gateway.Router
	executor      *gateway.StepExecutor
	mcpServer     *mcp.Server
	vaultConn     *vault.Vault
	cfg           *config.Config
	mux           *http.ServeMux
	soManager     *structuredoutput.Manager
	eventHub      *Hub
	perm          *permission.Engine
	runsMu        sync.Mutex
	activeRuns    map[uuid.UUID]stdcontext.CancelFunc
	startingRuns  map[uuid.UUID]startingRun
	contextCache  *context.CompilationCache
	shutdown      func()
	pickDirectory func(stdcontext.Context) (string, error)
}

func NewServer(
	pool *pgxpool.Pool,
	providerRepo *repos.ProviderRepo,
	routingRepo *repos.RoutingRuleRepo,
	sessionRepo *repos.SessionRepo,
	costLogRepo *repos.CostLogRepo,
	budgetRepo *repos.BudgetRepo,
	auditRepo *repos.AuditRepo,
	executionRepo *repos.ExecutionLogRepo,
	approvalGateRepo *repos.ApprovalGateRepo,
	searcher *context.Searcher,
	indexer *context.Indexer,
	router *gateway.Router,
	executor *gateway.StepExecutor,
	mcpServer *mcp.Server,
	vaultConn *vault.Vault,
	cfg *config.Config,
	soManager *structuredoutput.Manager,
) *Server {
	if cfg == nil {
		cfg = config.Default()
	}
	cfg.Server.StartTime = time.Now()

	s := &Server{
		pool:          pool,
		providers:     providerRepo,
		routingRules:  routingRepo,
		sessions:      sessionRepo,
		tasks:         repos.NewTaskRepo(pool),
		costLogs:      costLogRepo,
		budgets:       budgetRepo,
		audit:         auditRepo,
		execution:     executionRepo,
		approvalGates: approvalGateRepo,
		costTracker:   cost.NewTracker(costLogRepo, budgetRepo, auditRepo),
		searcher:      searcher,
		indexer:       indexer,
		router:        router,
		executor:      executor,
		mcpServer:     mcpServer,
		vaultConn:     vaultConn,
		cfg:           cfg,
		mux:           http.NewServeMux(),
		soManager:     soManager,
		eventHub:      NewHub(pool),
		perm:          permission.New(),
		activeRuns:    map[uuid.UUID]stdcontext.CancelFunc{},
		startingRuns:  map[uuid.UUID]startingRun{},
		contextCache:  context.NewCompilationCache(256, 10*time.Minute),
		pickDirectory: nativepicker.PickDirectory,
	}
	s.addDefaultPermissionRules()
	_ = s.reconcileResolvedRunLifecycle(stdcontext.Background())
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	handler := http.Handler(s.mux)
	handler = RequestHardening(handler)
	if s.cfg.Auth.Mode != "none" {
		handler = LocalAuth(s.cfg.Auth.Token, handler)
	}
	handler = LocalRateLimit(120, time.Minute, handler)
	return RequestID(Logger(Recoverer(SecurityHeaders(LocalHostOnly(CORS(EndpointTimeout(60*time.Second, handler)))))))
}

// SetShutdown wires process lifecycle behavior without coupling embedded tests
// or alternative hosts to OS signals.
func (s *Server) SetShutdown(shutdown func()) {
	s.shutdown = shutdown
}

func (s *Server) handleShutdown(w http.ResponseWriter, _ *http.Request) {
	if s.shutdown == nil {
		respondError(w, http.StatusServiceUnavailable, "SHUTDOWN_UNAVAILABLE", "graceful shutdown is not configured", nil)
		return
	}
	respondJSON(w, http.StatusAccepted, map[string]string{"status": "shutting_down"})
	go s.shutdown()
}

func (s *Server) registerRoutes() {
	// Health
	s.mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	s.mux.HandleFunc("POST /api/v1/system/shutdown", s.handleShutdown)
	s.mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("GET /api/v1/semantic-search", s.handleGetSemanticSearch)
	s.mux.HandleFunc("POST /api/v1/semantic-search/migrate", s.handleMigrateSemanticSearch)

	// Workspaces / projects
	s.mux.HandleFunc("GET /api/v1/workspaces", s.handleListWorkspaces)
	s.mux.HandleFunc("POST /api/v1/workspaces", s.handleCreateWorkspace)
	s.mux.HandleFunc("PUT /api/v1/workspaces/{id}", s.handleUpdateWorkspace)
	s.mux.HandleFunc("DELETE /api/v1/workspaces/{id}", s.handleDeleteWorkspace)
	s.mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	s.mux.HandleFunc("POST /api/v1/projects", s.handleCreateProject)
	s.mux.HandleFunc("POST /api/v1/projects/create-new", s.handleCreateNewProject)
	s.mux.HandleFunc("POST /api/v1/projects/pick-directory", s.handlePickProjectDirectory)
	s.mux.HandleFunc("POST /api/v1/projects/pick-parent-directory", s.handlePickParentDirectory)
	s.mux.HandleFunc("DELETE /api/v1/projects/{id}", s.handleDeleteProject)
	s.mux.HandleFunc("GET /api/v1/projects/{id}/code-index", s.handleGetProjectCodeIndex)
	s.mux.HandleFunc("POST /api/v1/projects/{id}/code-index/reindex", s.handleReindexProjectCode)
	s.mux.HandleFunc("GET /api/v1/projects/{id}/code-map/nodes", s.handleFindCodeMapNodes)
	s.mux.HandleFunc("GET /api/v1/projects/{id}/code-map/neighborhood", s.handleCodeMapNeighborhood)
	s.mux.HandleFunc("GET /api/v1/repo/analyze", s.handleAnalyzeRepository)
	s.mux.HandleFunc("GET /api/v1/repo/search", s.handleSearchRepository)

	// Providers
	s.mux.HandleFunc("GET /api/v1/providers", s.handleListProviders)
	s.mux.HandleFunc("POST /api/v1/providers", s.handleCreateProvider)
	s.mux.HandleFunc("GET /api/v1/providers/{id}", s.handleGetProvider)
	s.mux.HandleFunc("PUT /api/v1/providers/{id}", s.handleUpdateProvider)
	s.mux.HandleFunc("DELETE /api/v1/providers/{id}", s.handleDeleteProvider)
	s.mux.HandleFunc("POST /api/v1/providers/{id}/test", s.handleTestProvider)
	s.mux.HandleFunc("POST /api/v1/providers/{id}/fetch-models", s.handleFetchProviderModels)
	s.mux.HandleFunc("GET /api/v1/providers/discover-local", s.handleDiscoverLocalProviders)

	// Routing rules
	s.mux.HandleFunc("GET /api/v1/routing-rules", s.handleListRoutingRules)
	s.mux.HandleFunc("POST /api/v1/routing-rules", s.handleCreateRoutingRule)
	s.mux.HandleFunc("GET /api/v1/routing-rules/{id}", s.handleGetRoutingRule)
	s.mux.HandleFunc("PUT /api/v1/routing-rules/{id}", s.handleUpdateRoutingRule)
	s.mux.HandleFunc("DELETE /api/v1/routing-rules/{id}", s.handleDeleteRoutingRule)
	s.mux.HandleFunc("POST /api/v1/routing-rules/reorder", s.handleReorderRoutingRules)

	// Sessions / Tasks
	s.mux.HandleFunc("GET /api/v1/tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /api/v1/tasks", s.handleCreateTask)
	s.mux.HandleFunc("GET /api/v1/tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("GET /api/v1/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("GET /api/v1/runs/{id}/events", s.handleRunEvents)
	s.mux.HandleFunc("GET /api/v1/runs/{id}/export", s.handleExportRun)
	s.mux.HandleFunc("GET /api/v1/runs/{id}/promotion-readiness", s.handlePromotionReadiness)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/open-ide", s.handleOpenRunInIDE)
	s.mux.HandleFunc("GET /api/v1/system/launchers", s.handleListLaunchers)
	s.mux.HandleFunc("GET /api/v1/runs", s.handleListRuns)
	s.mux.HandleFunc("POST /api/v1/runs/{id}/outcome", s.handleRunOutcome)
	s.mux.HandleFunc("GET /api/v1/metrics/outcomes", s.handleOutcomeMetrics)
	s.mux.HandleFunc("PUT /api/v1/tasks/{id}", s.handleUpdateTask)
	s.mux.HandleFunc("DELETE /api/v1/tasks/{id}", s.handleDeleteTask)
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/run", s.handleRunTask)
	s.mux.HandleFunc("POST /api/v1/tasks/{id}/cancel", s.handleCancelTask)
	s.mux.HandleFunc("GET /api/v1/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /api/v1/sessions/{id}", s.handleGetSession)

	s.mux.HandleFunc("GET /api/v1/context/index-status", func(w http.ResponseWriter, r *http.Request) {
		if s.indexer == nil {
			respondJSON(w, http.StatusOK, map[string]any{"schema_version": "1", "fresh": false, "indexed_files": 0})
			return
		}
		respondJSON(w, http.StatusOK, s.indexer.Status())
	})

	// Approval gates
	s.mux.HandleFunc("GET /api/v1/approval-gates", s.handleListApprovalGates)
	s.mux.HandleFunc("POST /api/v1/approval-gates", s.handleCreateApprovalGate)
	s.mux.HandleFunc("PUT /api/v1/approval-gates/{id}", s.handleUpdateApprovalGate)
	s.mux.HandleFunc("DELETE /api/v1/approval-gates/{id}", s.handleDeleteApprovalGate)
	s.mux.HandleFunc("POST /api/v1/approval-gates/check", s.handleCheckApprovalGate)
	s.mux.HandleFunc("POST /api/v1/approvals/resolve", s.handleResolveApproval)

	// Costs
	s.mux.HandleFunc("GET /api/v1/costs", s.handleGetCostSummary)
	s.mux.HandleFunc("GET /api/v1/costs/logs", s.handleListCostLogs)
	s.mux.HandleFunc("POST /api/v1/costs/record", s.handleRecordCall)
	s.mux.HandleFunc("POST /api/v1/costs/simulate", s.handleSimulateCosts)
	s.mux.HandleFunc("POST /api/v1/costs/estimate", s.handleCostEstimate)

	// Budgets
	s.mux.HandleFunc("GET /api/v1/budgets", s.handleListBudgets)
	s.mux.HandleFunc("POST /api/v1/budgets", s.handleCreateBudget)
	s.mux.HandleFunc("GET /api/v1/budgets/{id}", s.handleGetBudget)
	s.mux.HandleFunc("PUT /api/v1/budgets/{id}", s.handleUpdateBudget)
	s.mux.HandleFunc("DELETE /api/v1/budgets/{id}", s.handleDeleteBudget)

	// Config
	s.mux.HandleFunc("GET /api/v1/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/v1/config", s.handleUpdateConfig)
	s.mux.HandleFunc("POST /api/v1/config/validate", s.handleValidateConfig)

	// Status
	s.mux.HandleFunc("GET /api/v1/status", s.handleGetStatus)

	// Costs projection
	s.mux.HandleFunc("GET /api/v1/costs/projection", s.handleGetCostProjection)

	// Audit
	s.mux.HandleFunc("GET /api/v1/audit-log", s.handleListAuditLog)
	s.mux.HandleFunc("POST /api/v1/audit-log/terminal", s.handleLogTerminalCommand)

	// Secrets
	s.mux.HandleFunc("POST /api/v1/secrets/scan", s.handleScanSecrets)

	// Vault
	s.mux.HandleFunc("GET /api/v1/vault/status", s.handleGetVaultStatus)
	s.mux.HandleFunc("GET /api/v1/vault/stats", s.handleGetVaultStats)
	s.mux.HandleFunc("GET /api/v1/vault/notes", s.handleListVaultNotes)
	s.mux.HandleFunc("POST /api/v1/vault/notes", s.handleCreateVaultNote)
	s.mux.HandleFunc("PUT /api/v1/vault/notes/{path...}", s.handleUpdateVaultNote)
	s.mux.HandleFunc("DELETE /api/v1/vault/notes/{path...}", s.handleDeleteVaultNote)
	s.mux.HandleFunc("GET /api/v1/vault/check", s.handleCheckVault)
	s.mux.HandleFunc("GET /api/v1/vault/search", s.handleSearchVault)
	s.mux.HandleFunc("POST /api/v1/vault/reindex", s.handleReindexVault)
	s.mux.HandleFunc("GET /api/v1/memory/candidates", s.handleListMemoryCandidates)
	s.mux.HandleFunc("POST /api/v1/memory/candidates/{id}/decision", s.handleDecideMemoryCandidate)

	// WebSocket
	s.mux.HandleFunc("GET /ws/v1/events", s.handleWebSocket)
}

func (s *Server) addDefaultPermissionRules() {
	worktreePattern := "**data" + string(filepath.Separator) + "worktrees**"
	highRisk := "high"
	s.perm.AddRule(permission.Rule{Name: "high-risk-write-approval", Priority: 200, Operation: "file.write", TargetPattern: "*", RepoPattern: &worktreePattern, Risk: &highRisk, Decision: permission.Ask})
	s.perm.AddRule(permission.Rule{Name: "isolated-worktree-write", Priority: 100, Operation: "file.write", TargetPattern: "*", RepoPattern: &worktreePattern, Decision: permission.Allow})
	for _, command := range []string{"git diff --check", "go test**", "go vet**", "npm test**", "npm run test**", "npm run typecheck**", "npm run build**", "cargo test**", "python -m pytest**", "pytest**"} {
		s.perm.AddRule(permission.Rule{Name: "safe-verification-" + command, Priority: 100, Operation: "command.exec", TargetPattern: command, RepoPattern: &worktreePattern, Decision: permission.Allow})
	}
}

func (s *Server) checkPerm(op, target, repoPath string) error {
	req := permission.Request{Operation: op, Target: target, RepoPath: repoPath}
	d, r := s.perm.Evaluate(req)
	switch d {
	case permission.Deny:
		reason := "permission denied"
		if r != nil {
			reason = r.Name
		}
		return fmt.Errorf("%s: %s on %s", reason, op, target)
	case permission.Ask:
		return fmt.Errorf("permission requires approval: %s on %s", op, target)
	default:
		return nil
	}
}
