//go:build e2e

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/llm"
)

type interruptedProvider struct {
	entered chan struct{}
	once    sync.Once
}

func (p *interruptedProvider) Name() string { return "interrupted-fixture" }
func (p *interruptedProvider) Chat(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	p.once.Do(func() { close(p.entered) })
	<-ctx.Done()
	return nil, ctx.Err()
}
func (p *interruptedProvider) ChatStream(ctx context.Context, r llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	_, err := p.Chat(ctx, r)
	return nil, err
}

// This is deliberately sequential: every iteration owns a real PostgreSQL
// child, repository, vault and HTTP server. No installed daemon is contacted.
func TestResilienceDatabaseRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded database E2E")
	}
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	embedded, err := db.StartEmbedded(filepath.Join(root, "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if embedded != nil {
			stopResilienceDatabase(t, embedded)
		}
	})
	sqlDB, err := sql.Open("pgx", embedded.DSN)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.RunMigrations(sqlDB); err != nil {
		t.Fatal(err)
	}
	_ = sqlDB.Close()
	pool, err := pgxpool.New(context.Background(), embedded.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	repository := filepath.Join(root, "fixture")
	if err = os.MkdirAll(repository, 0755); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, repository, "hello.txt", "old\n")
	gitFixture(t, repository, "init", "--initial-branch=main")
	gitFixture(t, repository, "config", "user.email", "resilience@example.test")
	gitFixture(t, repository, "config", "user.name", "Resilience fixture")
	gitFixture(t, repository, "add", ".")
	gitFixture(t, repository, "commit", "-m", "fixture")
	record := &repos.Provider{Name: "fixture", ProviderType: "custom", DefaultModel: "fake-model", Models: "fake-model", IsActive: true}
	if err = repos.NewProviderRepo(pool).Create(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	blocked := &interruptedProvider{entered: make(chan struct{})}
	makeServer := func(provider llm.Provider) (*Server, *httptest.Server) {
		cfg := config.Default()
		cfg.Auth.Mode = "none"
		v := vault.New(filepath.Join(root, "vault"))
		if err := v.Ensure(); err != nil {
			t.Fatal(err)
		}
		s := NewServer(pool, repos.NewProviderRepo(pool), repos.NewRoutingRuleRepo(pool), repos.NewSessionRepo(pool), repos.NewCostLogRepo(pool), repos.NewBudgetRepo(pool), repos.NewAuditRepo(pool), repos.NewExecutionLogRepo(pool), repos.NewApprovalGateRepo(pool), nil, nil, nil, gateway.NewStepExecutor(map[string]llm.Provider{record.ID.String(): provider}, gateway.ExecutorConfig{DefaultTimeout: time.Minute, MaxRetries: 0}), nil, v, cfg, nil)
		h := httptest.NewServer(s.Handler())
		t.Cleanup(func() {
			s.runsMu.Lock()
			for _, cancel := range s.activeRuns {
				cancel()
			}
			s.runsMu.Unlock()
			waitResilienceIdle(t, s)
			h.Close()
		})
		return s, h
	}
	server, httpServer := makeServer(blocked)
	workspace := postData(t, httpServer.URL+"/api/v1/workspaces", map[string]any{"name": "resilience"})
	project := postData(t, httpServer.URL+"/api/v1/projects", map[string]any{"name": "fixture", "path": repository, "workspace_id": workspace["id"]})
	task := postData(t, httpServer.URL+"/api/v1/tasks", map[string]any{"repository_id": project["id"], "title": "Replace old with new", "description": "Change hello.txt and verify with git diff --check", "task_type": "bug_fix"})
	started := postData(t, httpServer.URL+"/api/v1/tasks/"+task["id"].(string)+"/run", map[string]any{})
	runID := started["run_id"].(string)
	select {
	case <-blocked.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("provider entry barrier timed out")
	}
	t.Log("FAULT database shutdown while provider call is in flight")
	stopResilienceDatabase(t, embedded)
	embedded = nil
	server.runsMu.Lock()
	for _, cancel := range server.activeRuns {
		cancel()
	}
	server.runsMu.Unlock()
	waitResilienceIdle(t, server)
	httpServer.Close()
	pool.Close()
	embedded, err = db.StartEmbedded(filepath.Join(root, "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	pool, err = pgxpool.New(context.Background(), embedded.DSN)
	if err != nil {
		t.Fatal(err)
	}
	// Advance only this fixture's lease, not wall-clock time or production data.
	if _, err = pool.Exec(context.Background(), `UPDATE task_runs SET lease_expires_at=NOW()-INTERVAL '1 minute' WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	fake := &durableFakeProvider{responses: []*llm.ChatResponse{
		toolResponse("patch", `{"path":"hello.txt","old_text":"old","new_text":"new"}`),
		toolResponse("command", `{"program":"git","args":["diff","--check"]}`),
		toolResponse("finish", `{"summary":"Updated hello.txt and checked the diff."}`),
	}}
	server, httpServer = makeServer(fake)
	for range 2 {
		if err = server.ReconcileInterruptedRuns(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var state, retained string
	var count int
	if err = pool.QueryRow(context.Background(), `SELECT state,recovery_count,worktree_path FROM task_runs WHERE id=$1`, runID).Scan(&state, &count, &retained); err != nil {
		t.Fatal(err)
	}
	if state != "interrupted" || count != 1 {
		t.Fatalf("recovery state=%s count=%d", state, count)
	}
	if _, err = os.Stat(retained); err != nil {
		t.Fatalf("lost interrupted worktree: %v", err)
	}
	events := getData(t, httpServer.URL+"/api/v1/runs/"+runID+"/events?after=0").(map[string]any)["events"].([]any)
	if len(events) == 0 {
		t.Fatal("reconnect lost durable events")
	}
	last := events[len(events)-1].(map[string]any)["sequence"]
	tail := getData(t, httpServer.URL+"/api/v1/runs/"+runID+"/events?after="+jsonNumber(last)).(map[string]any)["events"].([]any)
	if len(tail) != 0 {
		t.Fatal("cursor replay duplicated events")
	}
	t.Log("RECOVERY fresh server reconciles once; reconnect cursor preserved")
	next := postData(t, httpServer.URL+"/api/v1/tasks/"+task["id"].(string)+"/run", map[string]any{})
	nextID := next["run_id"].(string)
	if nextID == runID {
		t.Fatal("retry reused interrupted run")
	}
	waitForRunState(t, httpServer.URL, nextID, "review")
	waitResilienceIdle(t, server)
	// Deterministic contention, including an independently held database session.
	guard, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = guard.Exec(context.Background(), `SELECT pg_advisory_lock(hashtextextended($1,45))`, nextID); err != nil {
		guard.Release()
		t.Fatal(err)
	}
	status := resilienceOutcome(httpServer.URL, nextID, "rejected")
	_, unlockErr := guard.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,45))`, nextID)
	guard.Release()
	if unlockErr != nil {
		t.Fatal(unlockErr)
	}
	if status != 409 {
		t.Fatalf("contending decision status=%d", status)
	}
	start := make(chan struct{})
	statuses := make(chan int, 2)
	for range 2 {
		go func() { <-start; statuses <- resilienceOutcome(httpServer.URL, nextID, "rejected") }()
	}
	close(start)
	one, two := <-statuses, <-statuses
	if !((one == 200 && two == 409) || (one == 409 && two == 200)) {
		t.Fatalf("concurrent decisions: %d %d", one, two)
	}
	if err = pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM run_events WHERE run_id=$1 AND event_type='outcome_recorded'`, nextID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("outcome count=%d err=%v", count, err)
	}
	content, err := os.ReadFile(filepath.Join(repository, "hello.txt"))
	if err != nil || string(content) != "old\n" {
		t.Fatal("rejection changed primary checkout")
	}
	command := exec.Command("git", "worktree", "list", "--porcelain")
	command.Dir = repository
	listed, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	// The interrupted worktree is intentionally retained for recovery/audit.
	// Rejected worktrees must be gone; no third registry entry may survive.
	if strings.Count(string(listed), "worktree ") != 2 {
		t.Fatalf("unexpected residual worktrees:\n%s", listed)
	}
	gitFixture(t, repository, "worktree", "remove", "--force", retained)
	command = exec.Command("git", "worktree", "list", "--porcelain")
	command.Dir = repository
	listed, err = command.Output()
	if err != nil || strings.Count(string(listed), "worktree ") != 1 {
		t.Fatalf("fixture cleanup leaked a worktree: %s (%v)", listed, err)
	}
	t.Log("CLEANUP no active runs; rejected worktree gone; retained recovery fixture explicitly removed")
}

func jsonNumber(value any) string { data, _ := json.Marshal(value); return string(data) }
func resilienceOutcome(base, id, outcome string) int {
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Post(base+"/api/v1/runs/"+id+"/outcome", "application/json", bytes.NewBufferString(`{"outcome":"`+outcome+`"}`))
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	return response.StatusCode
}
func waitResilienceIdle(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s.runsMu.Lock()
		n := len(s.activeRuns) + len(s.startingRuns)
		s.runsMu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime worker did not terminate")
}
func stopResilienceDatabase(t *testing.T, embedded *db.Embedded) {
	t.Helper()
	if err := embedded.Stop(); err != nil {
		t.Errorf("database cleanup: %v", err)
	}
	address, err := url.Parse(embedded.DSN)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", address.Host, 500*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Error("database listener survived shutdown")
	}
}
