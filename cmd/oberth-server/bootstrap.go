package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/api"
	semcontext "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/internal/providersecret"
	"github.com/saterdoe/oberth/internal/structuredoutput"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/llm"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func run() {
	cfgPath := flag.String("config", "", "path to configuration file")
	migrateOnly := flag.Bool("migrate", false, "run database migrations and exit")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	configureLogger(cfg.Server.LogLevel)

	slog.Info("starting oberth server",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"log_level", cfg.Server.LogLevel,
	)

	// 3. Connect to database
	ctx := context.Background()
	var embeddedDB *db.Embedded
	if strings.EqualFold(cfg.Database.Driver, "embedded") || strings.TrimSpace(cfg.Database.DSN) == "" {
		var embeddedErr error
		embeddedDB, embeddedErr = db.StartEmbedded("data/embedded-postgres")
		if embeddedErr != nil {
			slog.Error("failed to start embedded database", "error", embeddedErr)
			os.Exit(1)
		}
		defer embeddedDB.Stop()
		cfg.Database.DSN = embeddedDB.DSN
		slog.Info("embedded single-user database started")
	}
	pool, err := db.Connect(ctx, cfg.Database.DSN)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// 4. Run migrations (using a *sql.DB for the migrate library)
	migrateDB, err := sql.Open("pgx", cfg.Database.DSN)
	if err != nil {
		slog.Error("failed to open database for migrations", "error", err)
		os.Exit(1)
	}
	defer migrateDB.Close()

	if err := db.RunMigrations(migrateDB); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations completed")

	if *migrateOnly {
		slog.Info("migrations completed successfully")
		return
	}

	// 5. Initialize repositories
	providerRepo := repos.NewProviderRepo(pool)
	routingRepo := repos.NewRoutingRuleRepo(pool)
	sessionRepo := repos.NewSessionRepo(pool)
	costLogRepo := repos.NewCostLogRepo(pool)
	budgetRepo := repos.NewBudgetRepo(pool)
	auditRepo := repos.NewAuditRepo(pool)
	executionRepo := repos.NewExecutionLogRepo(pool)
	approvalGateRepo := repos.NewApprovalGateRepo(pool)

	// 6. Initialize vault connector
	vaultConn := vault.New(cfg.Vault.Path)
	if err := vaultConn.Ensure(); err != nil {
		slog.Error("failed to initialize vault", "path", cfg.Vault.Path, "error", err)
		os.Exit(1)
	}
	slog.Info("vault initialized", "path", cfg.Vault.Path)

	embedder, dimensions := buildEmbedder(cfg)
	vectorStore := buildVectorStore(cfg, dimensions)

	// 9. Initialize LLM providers
	llmProviders := make(map[string]llm.Provider)
	dbProviders, err := providerRepo.List(ctx)
	if err != nil {
		slog.Warn("failed to list providers from database", "error", err)
	} else {
		for _, p := range dbProviders {
			if !p.IsActive {
				continue
			}
			if p.APIKeyEncrypted != nil && *p.APIKeyEncrypted != "" && !providersecret.IsSealed(*p.APIKeyEncrypted) {
				sealed, sealErr := providersecret.Seal(cfg.Auth.Token, *p.APIKeyEncrypted)
				if sealErr != nil {
					slog.Warn("failed to protect legacy provider credential", "name", p.Name, "error", sealErr)
					continue
				}
				p.APIKeyEncrypted = &sealed
				if updateErr := providerRepo.Update(ctx, &p); updateErr != nil {
					slog.Warn("failed to persist protected provider credential", "name", p.Name, "error", updateErr)
					continue
				}
				slog.Info("protected legacy provider credential", "name", p.Name)
			}
			decrypted, err := providerWithOpenSecret(cfg.Auth.Token, p)
			if err != nil {
				slog.Warn("failed to decrypt llm provider credential", "name", p.Name, "type", p.ProviderType, "error", err)
				continue
			}
			provider, err := gateway.BuildProvider(decrypted)
			if err != nil {
				slog.Warn("failed to register llm provider", "name", p.Name, "type", p.ProviderType, "error", err)
				continue
			}
			llmProviders[p.ID.String()] = provider
			slog.Info("llm provider registered", "name", p.Name, "type", p.ProviderType)
		}
	}
	slog.Info("llm providers initialized", "count", len(llmProviders))

	// 9. Initialize structured output engine (BAML / native JSON)
	var firstProvider llm.Provider
	for _, p := range llmProviders {
		firstProvider = p
		break
	}
	soManager, err := structuredoutput.NewManager(cfg.StructuredOutputs, firstProvider)
	if err != nil {
		slog.Warn("failed to initialize structured output engine", "error", err)
	} else if soManager.Enabled {
		slog.Info("structured output engine initialized", "engine", soManager.EngineName)
	} else {
		slog.Info("structured output engine disabled")
	}

	// 10. Initialize gateway (router, executor, reviewer)
	routingRouter := gateway.NewRouter(routingRepo, providerRepo)
	attemptTimeout := 5 * time.Minute
	if configured := strings.TrimSpace(cfg.LLM.AttemptTimeout); configured != "" {
		parsed, parseErr := time.ParseDuration(configured)
		if parseErr != nil || parsed <= 0 {
			slog.Warn("invalid llm attempt timeout; using default", "configured", configured, "default", attemptTimeout)
		} else {
			attemptTimeout = parsed
		}
	}
	executor := gateway.NewStepExecutor(llmProviders, gateway.ExecutorConfig{
		DefaultTimeout: attemptTimeout,
		ResolveProvider: func(ctx context.Context, providerID string) (llm.Provider, error) {
			id, err := uuid.Parse(providerID)
			if err != nil {
				return nil, err
			}
			p, err := providerRepo.GetByID(ctx, id)
			if err != nil {
				return nil, err
			}
			if !p.IsActive {
				return nil, fmt.Errorf("provider %q is inactive", providerID)
			}
			decrypted, err := providerWithOpenSecret(cfg.Auth.Token, *p)
			if err != nil {
				return nil, err
			}
			return gateway.BuildProvider(decrypted)
		},
		OnFallback: func(ctx context.Context, event gateway.FallbackEvent) {
			var sessionID *uuid.UUID
			if sid := gateway.AuditSessionID(ctx); sid != "" {
				if parsed, err := uuid.Parse(sid); err == nil {
					sessionID = &parsed
				}
			}
			details := map[string]any{
				"step_id":       event.StepID,
				"from_provider": event.FromProvider,
				"from_model":    event.FromModel,
				"to_provider":   event.ToProvider,
				"to_model":      event.ToModel,
				"attempt":       event.Attempt,
				"error":         event.Error,
			}
			data, _ := json.Marshal(details)
			if err := auditRepo.Create(ctx, &repos.AuditLogEntry{
				SessionID: sessionID,
				Action:    "llm_fallback",
				Actor:     "agent:gateway",
				Details:   data,
			}); err != nil {
				slog.Warn("failed to write fallback audit log", "error", err)
			}
		},
		OnTimeoutDecision: func(ctx context.Context, event gateway.TimeoutDecision) {
			var sessionID *uuid.UUID
			if sid := gateway.AuditSessionID(ctx); sid != "" {
				if parsed, err := uuid.Parse(sid); err == nil {
					sessionID = &parsed
				}
			}
			details := map[string]any{
				"step_id": event.StepID, "provider_id": event.ProviderID,
				"provider": event.Provider, "model": event.Model,
				"decision": event.Decision, "reason": event.Reason,
				"elapsed_ms": event.Elapsed.Milliseconds(), "deadline_ms": event.Deadline.Milliseconds(),
			}
			data, _ := json.Marshal(details)
			if err := auditRepo.Create(ctx, &repos.AuditLogEntry{
				SessionID: sessionID,
				Action:    "llm_timeout_decision",
				Actor:     "agent:adaptive-timeout",
				Details:   data,
			}); err != nil {
				slog.Warn("failed to write timeout decision audit log", "error", err)
			}
		},
	})
	reviewer := gateway.NewReviewer(executor)
	_ = reviewer

	// 12. Initialize semantic search + indexer
	activeVectorStore := vectorStore
	if cfg.VectorDB.Engine == "disabled" {
		activeVectorStore = nil
	}
	searcher := semcontext.NewSearcherWithEmbedder(vaultConn, activeVectorStore, embedder)
	var indexer *semcontext.Indexer
	if vectorStore != nil && embedder != nil {
		indexer = semcontext.NewIndexerWithEmbedder(vaultConn, vectorStore, embedder)
		slog.Info("semantic search initialized", "embedder", embedder.Fingerprint(), "dimensions", embedder.Dimensions())
	}
	pipeline := semcontext.NewPipeline(vaultConn, searcher)

	// 12. Auto-index vault on startup (if enabled)
	if cfg.Vault.AutoIndex && indexer != nil && cfg.VectorDB.Engine != "disabled" {
		go func() {
			slog.Info("starting initial vault reindex")
			var result *semcontext.IndexResult
			var err error
			if cfg.VectorDB.Engine == "qdrant" {
				// A persisted Qdrant collection is the last verified generation.
				// Repopulate it idempotently instead of deleting it during startup.
				result, err = indexer.ReindexIncremental(ctx)
			} else {
				result, err = indexer.Reindex(ctx)
			}
			if err != nil {
				slog.Error("initial vault reindex failed", "error", err)
			} else {
				slog.Info("initial vault reindex completed", "chunks", result.IndexedChunks, "notes", result.TotalNotes, "duration_ms", result.DurationMs)
			}
		}()

		interval, err := time.ParseDuration(cfg.Vault.IndexInterval)
		if err != nil {
			slog.Warn("invalid vault index interval, skipping periodic reindex", "interval", cfg.Vault.IndexInterval, "error", err)
		} else {
			go func() {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						slog.Info("running periodic vault reindex")
						if result, err := indexer.ReindexIncremental(ctx); err != nil {
							slog.Error("periodic reindex failed", "error", err)
						} else {
							slog.Info("periodic reindex completed", "chunks", result.IndexedChunks, "notes", result.TotalNotes)
						}
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	}

	mcpServer := buildMCPServer(vaultConn, searcher, pipeline)

	// 13. Initialize API server
	apiServer := api.NewServer(
		pool,
		providerRepo,
		routingRepo,
		sessionRepo,
		costLogRepo,
		budgetRepo,
		auditRepo,
		executionRepo,
		approvalGateRepo,
		searcher,
		indexer,
		routingRouter,
		executor,
		mcpServer,
		vaultConn,
		cfg,
		soManager,
	)
	if err := apiServer.ReconcileInterruptedRuns(ctx); err != nil {
		slog.Warn("failed to reconcile interrupted runs", "error", err)
	}
	if err := apiServer.ReconcileTerminalWorktrees(ctx); err != nil {
		slog.Warn("failed to reconcile terminal worktrees", "error", err)
	}

	if err := serve(ctx, cfg, apiServer); err != nil {
		slog.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}
