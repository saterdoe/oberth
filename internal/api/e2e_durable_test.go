//go:build e2e

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/llm"
)

type durableFakeProvider struct {
	mu        sync.Mutex
	responses []*llm.ChatResponse
}

type durableFailingProvider struct{}

func (*durableFailingProvider) Name() string { return "durable-failing" }
func (*durableFailingProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, fmt.Errorf("fixture provider unavailable")
}
func (*durableFailingProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, fmt.Errorf("fixture provider unavailable")
}

func (p *durableFakeProvider) Name() string { return "durable-fake" }

func (p *durableFakeProvider) append(responses ...*llm.ChatResponse) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses = append(p.responses, responses...)
}

func (p *durableFakeProvider) Chat(_ context.Context, request llm.ChatRequest) (*llm.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(request.Tools) == 0 {
		return nil, fmt.Errorf("typed tools are required")
	}
	if len(p.responses) == 0 {
		return nil, fmt.Errorf("unexpected provider call")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *durableFakeProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, fmt.Errorf("streaming is not expected when typed tools are present")
}

func TestDurableRunHTTPHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded database E2E")
	}
	controlRoot := t.TempDir()
	previousCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(controlRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousCWD) })

	embedded, err := db.StartEmbedded(filepath.Join(controlRoot, "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = embedded.Stop() })
	sqlDB, err := sql.Open("pgx", embedded.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), embedded.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repository := filepath.Join(controlRoot, "fixture")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, repository, "go.mod", "module example.com/durable\n\ngo 1.24\n")
	writeFixtureFile(t, repository, "main.go", "package durable\n\nfunc Message() string { return \"old\" }\n")
	gitFixture(t, repository, "init", "--initial-branch=main")
	gitFixture(t, repository, "config", "user.email", "e2e@example.test")
	gitFixture(t, repository, "config", "user.name", "oberth E2E")
	gitFixture(t, repository, "add", ".")
	gitFixture(t, repository, "commit", "-m", "fixture")

	providers := repos.NewProviderRepo(pool)
	routing := repos.NewRoutingRuleRepo(pool)
	sessions := repos.NewSessionRepo(pool)
	costLogs := repos.NewCostLogRepo(pool)
	budgets := repos.NewBudgetRepo(pool)
	audit := repos.NewAuditRepo(pool)
	executions := repos.NewExecutionLogRepo(pool)
	approvals := repos.NewApprovalGateRepo(pool)
	providerRecord := &repos.Provider{
		Name: "fake", ProviderType: "custom", DefaultModel: "fake-model",
		Models: "fake-model", IsActive: true,
	}
	if err := providers.Create(context.Background(), providerRecord); err != nil {
		t.Fatal(err)
	}
	fake := &durableFakeProvider{responses: []*llm.ChatResponse{
		toolResponse("read", `{"path":"main.go"}`),
		toolResponse("patch", `{"path":"main.go","old_text":"return \"old\"","new_text":"return \"new\""}`),
		toolResponse("command", `{"program":"go","args":["test","./..."]}`),
		toolResponse("record_reasoning", `{"experiment":{"id":"x-message","question":"Does the candidate preserve the module contract?","preconditions":["main.go contains the candidate change"],"environment":"isolated worktree","command":"go test ./...","expectation":"all packages pass","observation":"all packages passed","status":"passed","duration_ms":1,"cost":0,"evidence_ids":["ev-turn-003"],"claim_ids":["p-message"],"baseline_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","candidate_fingerprint":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}`),
		toolResponse("record_reasoning", `{"record":{"id":"p-message","kind":"property","statement":"The module passes its tests after changing Message","status":"passed","required":true,"evidence_ids":["ev-turn-003"]}}`),
		toolResponse("finish", `{"summary":"Updated Message and verified the module."}`),
	}}
	executor := gateway.NewStepExecutor(map[string]llm.Provider{providerRecord.ID.String(): fake}, gateway.ExecutorConfig{
		DefaultTimeout: 15 * time.Second, MaxRetries: 0,
	})
	cfg := config.Default()
	cfg.Auth.Mode = "none"
	vaultRoot := filepath.Join(controlRoot, "vault")
	vaultConn := vault.New(vaultRoot)
	if err := vaultConn.Ensure(); err != nil {
		t.Fatal(err)
	}
	server := NewServer(pool, providers, routing, sessions, costLogs, budgets, audit, executions, approvals, nil, nil, nil, executor, nil, vaultConn, cfg, nil)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	if ready := server.runtimeReadiness(context.Background()); !ready.Ready {
		t.Fatalf("configured runtime not ready: %+v", ready)
	}
	readOnlyConfig := pool.Config().Copy()
	readOnlyConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readOnlyPool, err := pgxpool.NewWithConfig(context.Background(), readOnlyConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(readOnlyPool.Close)
	readOnlyServer := &Server{pool: readOnlyPool, executor: executor}
	if ready := readOnlyServer.runtimeReadiness(context.Background()); ready.Ready || ready.Reason != "database_not_writable" {
		t.Fatalf("read-only database accepted: %+v", ready)
	}

	t.Cleanup(func() {
		snapshot := server.telemetry.Snapshot()
		var contextSeen, providerSeen bool
		for _, metric := range snapshot.Metrics {
			contextSeen = contextSeen || metric.Stage == "context"
			providerSeen = providerSeen || metric.Stage == "provider.chat"
		}
		if !contextSeen || !providerSeen {
			t.Errorf("missing stage/provider telemetry: %+v", snapshot.Metrics)
		}
		for _, trace := range snapshot.Traces {
			if trace.RunID == "" || trace.TaskID == "" || trace.SessionID == "" {
				t.Errorf("uncorrelated trace: %+v", trace)
			}
		}
	})
	workspace := postData(t, httpServer.URL+"/api/v1/workspaces", map[string]any{"name": "e2e"})
	project := postData(t, httpServer.URL+"/api/v1/projects", map[string]any{
		"name": "fixture", "path": repository, "workspace_id": workspace["id"],
	})
	task := postData(t, httpServer.URL+"/api/v1/tasks", map[string]any{
		"repository_id": project["id"], "title": "Update message",
		"description": "Replace old with new and verify the module", "task_type": "bug_fix",
	})
	runURL := httpServer.URL + "/api/v1/tasks/" + task["id"].(string) + "/run"
	const runKey = "durable-happy-path"
	start := make(chan struct{})
	responses := make(chan map[string]any, 2)
	failures := make(chan error, 2)
	var requests sync.WaitGroup
	for range 2 {
		requests.Add(1)
		go func() {
			defer requests.Done()
			<-start
			data, requestErr := postDataWithKeyResult(runURL, map[string]any{}, runKey)
			if requestErr != nil {
				failures <- requestErr
				return
			}
			responses <- data
		}()
	}
	close(start)
	requests.Wait()
	close(responses)
	close(failures)
	for requestErr := range failures {
		t.Fatal(requestErr)
	}
	var runID string
	var sessionID string
	for accepted := range responses {
		current := accepted["run_id"].(string)
		if runID == "" {
			runID = current
			sessionID = accepted["session_id"].(string)
		} else if current != runID {
			t.Fatalf("simultaneous idempotent requests created different runs: %s and %s", runID, current)
		}
	}
	if runID == "" {
		t.Fatal("simultaneous run requests returned no run")
	}

	run := waitForRunState(t, httpServer.URL, runID, "review")
	bundle := run["result_bundle"].(map[string]any)
	if bundle["verification_status"] != "passed" {
		t.Fatalf("expected passed verification, got %+v", bundle["verification_status"])
	}
	happyReasoning := bundle["reasoning"].(map[string]any)
	happyAssessment := happyReasoning["assessment"].(map[string]any)
	if happyAssessment["coverage_percent"] != float64(100) ||
		len(happyAssessment["gate_blockers"].([]any)) != 0 {
		t.Fatalf("required property was not backed by automatic command evidence: %+v", happyReasoning)
	}
	happyEvidence := happyReasoning["evidence"].([]any)
	if len(happyEvidence) < 2 {
		t.Fatalf("read and command observations must create automatic evidence: %+v", happyEvidence)
	}
	happyExperiments := happyReasoning["experiments"].([]any)
	if len(happyExperiments) != 1 || happyExperiments[0].(map[string]any)["id"] != "x-message" {
		t.Fatalf("reproducible experiment was not preserved end-to-end: %+v", happyExperiments)
	}
	candidates := getData(t, httpServer.URL+"/api/v1/memory/candidates?status=pending")
	candidateItems := candidates.([]any)
	if len(candidateItems) != 2 {
		t.Fatalf("expected property and experiment memory candidates, got %+v", candidates)
	}
	for _, item := range candidateItems {
		candidate := item.(map[string]any)
		if candidate["validity_status"] != "current" || len(candidate["evidence_ids"].([]any)) == 0 {
			t.Fatalf("reasoning memory must preserve current validity and citations: %+v", candidate)
		}
	}
	candidateID := candidateItems[0].(map[string]any)["id"].(string)
	promotion := postData(t, httpServer.URL+"/api/v1/memory/candidates/"+candidateID+"/decision", map[string]any{"decision": "approved"})
	notePath, _ := promotion["note_path"].(string)
	if notePath == "" {
		t.Fatalf("approved candidate was not promoted: %+v", promotion)
	}
	if _, err := vaultConn.ReadNote(notePath); err != nil {
		t.Fatalf("promoted memory is not readable: %v", err)
	}
	if _, err := vaultConn.ReadNote("memory-index"); err != nil {
		t.Fatalf("memory index was not rebuilt: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(repository, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte(`return "new"`)) {
		t.Fatal("main checkout changed before explicit approval")
	}
	replayed := postDataWithKey(t, runURL, map[string]any{}, "durable-happy-path")
	if replayed["run_id"] != runID {
		t.Fatalf("idempotent retry created another run: %+v", replayed)
	}

	outcome := postData(t, httpServer.URL+"/api/v1/runs/"+runID+"/outcome", map[string]any{
		"outcome": "accepted", "note": "E2E reviewed",
	})
	if outcome["outcome"] != "accepted" {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	var acceptedRunState, acceptedTaskState, acceptedSessionState string
	if err := pool.QueryRow(context.Background(), `
		SELECT r.state,t.status,s.status
		FROM task_runs r
		JOIN tasks t ON t.id=r.task_id
		JOIN sessions s ON s.id=r.session_id
		WHERE r.id=$1`, runID).Scan(&acceptedRunState, &acceptedTaskState, &acceptedSessionState); err != nil {
		t.Fatal(err)
	}
	if acceptedRunState != "completed" || acceptedTaskState != "completed" || acceptedSessionState != "completed" {
		t.Fatalf("accepted run did not complete its lifecycle: run=%s task=%s session=%s", acceptedRunState, acceptedTaskState, acceptedSessionState)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE tasks SET status='review' WHERE id=$1`, task["id"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE sessions SET status='review',ended_at=NULL WHERE id=$1`, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := server.reconcileResolvedRunLifecycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT t.status,s.status
		FROM tasks t
		JOIN sessions s ON s.task_id=t.id
		WHERE t.id=$1 AND s.id=$2`, task["id"], sessionID).Scan(&acceptedTaskState, &acceptedSessionState); err != nil {
		t.Fatal(err)
	}
	if acceptedTaskState != "completed" || acceptedSessionState != "completed" {
		t.Fatalf("startup reconciliation left a resolved review stale: task=%s session=%s", acceptedTaskState, acceptedSessionState)
	}
	acceptedRun := getData(t, httpServer.URL+"/api/v1/runs/"+runID).(map[string]any)
	acceptedBundle := acceptedRun["result_bundle"].(map[string]any)
	if acceptedBundle["outcome"] != "accepted" {
		t.Fatalf("human outcome was not written back to the exportable bundle: %+v", acceptedBundle)
	}
	after, err := os.ReadFile(filepath.Join(repository, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(after, []byte(`return "new"`)) {
		t.Fatalf("approved change was not promoted:\n%s", after)
	}
	var costAuditEvents int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM audit_log WHERE action='cost.call_recorded' AND actor='system'`).Scan(&costAuditEvents); err != nil {
		t.Fatal(err)
	}
	if costAuditEvents != 6 {
		t.Fatalf("expected one cost audit event per model turn, got %d", costAuditEvents)
	}
	for _, update := range []struct {
		query string
		id    any
	}{
		{`UPDATE task_runs SET state='running',lease_expires_at=NOW()-INTERVAL '1 minute' WHERE id=$1`, runID},
		{`UPDATE tasks SET status='running' WHERE id=$1`, task["id"]},
		{`UPDATE sessions SET status='active',ended_at=NULL WHERE id=$1`, sessionID},
	} {
		if _, err := pool.Exec(context.Background(), update.query, update.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := server.ReconcileInterruptedRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	var recoveredRun, recoveredTask, recoveredSession, recoveredWorktree string
	var resumeFromSequence int64
	var recoveryCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT r.state,t.status,s.status,r.worktree_path,r.resume_from_sequence,r.recovery_count
		FROM task_runs r
		JOIN tasks t ON t.id=r.task_id
		JOIN sessions s ON s.id=r.session_id
		WHERE r.id=$1`, runID).Scan(&recoveredRun, &recoveredTask, &recoveredSession, &recoveredWorktree, &resumeFromSequence, &recoveryCount); err != nil {
		t.Fatal(err)
	}
	if recoveredRun != "interrupted" || recoveredTask != "blocked" || recoveredSession != "blocked" {
		t.Fatalf("unexpected recovered states: run=%s task=%s session=%s", recoveredRun, recoveredTask, recoveredSession)
	}
	if resumeFromSequence == 0 || recoveryCount != 1 {
		t.Fatalf("missing durable recovery checkpoint: sequence=%d count=%d", resumeFromSequence, recoveryCount)
	}
	if _, err := os.Stat(recoveredWorktree); err != nil {
		t.Fatalf("interrupted worktree was not preserved: %v", err)
	}
	var interruptionEvents int
	var artifactsPreserved bool
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*),COALESCE(bool_and((payload->>'artifacts_preserved')::boolean),false)
		FROM run_events WHERE run_id=$1 AND event_type='run_interrupted'`, runID).Scan(&interruptionEvents, &artifactsPreserved); err != nil {
		t.Fatal(err)
	}
	if interruptionEvents != 1 || !artifactsPreserved {
		t.Fatalf("unexpected interruption evidence: events=%d artifacts_preserved=%v", interruptionEvents, artifactsPreserved)
	}
	if err := server.ReconcileInterruptedRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT recovery_count,(SELECT COUNT(*) FROM run_events WHERE run_id=$1 AND event_type='run_interrupted')
		FROM task_runs WHERE id=$1`, runID).Scan(&recoveryCount, &interruptionEvents); err != nil {
		t.Fatal(err)
	}
	if recoveryCount != 1 || interruptionEvents != 1 {
		t.Fatalf("recovery was not idempotent: count=%d events=%d", recoveryCount, interruptionEvents)
	}

	fake.append(toolResponse("finish", `{"summary":"Claimed success without verification."}`))
	unverifiedTask := postData(t, httpServer.URL+"/api/v1/tasks", map[string]any{
		"repository_id": project["id"], "title": "Unverified change",
		"description": "Finish without running repository verification", "task_type": "bug_fix",
	})
	unverifiedRun := postData(t, httpServer.URL+"/api/v1/tasks/"+unverifiedTask["id"].(string)+"/run", map[string]any{})
	blocked := waitForRunState(t, httpServer.URL, unverifiedRun["run_id"].(string), "blocked")
	blockedBundle := blocked["result_bundle"].(map[string]any)
	if blockedBundle["verification_status"] != "not_run" {
		t.Fatalf("unverified run was not blocked with explicit evidence: %+v", blockedBundle)
	}

	fake.append(
		toolResponse("record_reasoning", `{"record":{"id":"u-production-retry","kind":"unknown","statement":"The deployed retry policy is unavailable","status":"unresolved","next_action":"provide the deployed retry configuration"}}`),
		toolResponse("stop_insufficient_evidence", `{"unknown_id":"u-production-retry","summary":"A safe retry change cannot be justified without the deployed policy."}`),
	)
	unknownTask := postData(t, httpServer.URL+"/api/v1/tasks", map[string]any{
		"repository_id": project["id"], "title": "Choose retry strategy",
		"description": "Change retries using the deployed production policy", "task_type": "architecture",
	})
	unknownRun := postData(t, httpServer.URL+"/api/v1/tasks/"+unknownTask["id"].(string)+"/run", map[string]any{})
	unknownBlocked := waitForRunState(t, httpServer.URL, unknownRun["run_id"].(string), "blocked")
	unknownBundle := unknownBlocked["result_bundle"].(map[string]any)
	if unknownBundle["verification_status"] != "not_run" {
		t.Fatalf("evidence stop must not manufacture verification: %+v", unknownBundle)
	}
	runtimeData := unknownBundle["runtime"].(map[string]any)
	if runtimeData["termination_reason"] != "insufficient_evidence" {
		t.Fatalf("missing legitimate termination reason: %+v", runtimeData)
	}
	reasoningData := unknownBundle["reasoning"].(map[string]any)
	records := reasoningData["records"].([]any)
	if len(records) != 1 || records[0].(map[string]any)["id"] != "u-production-retry" {
		t.Fatalf("unknown was not preserved in result bundle: %+v", reasoningData)
	}
	events := getData(t, httpServer.URL+"/api/v1/runs/"+unknownRun["run_id"].(string)+"/events?after=0").(map[string]any)["events"].([]any)
	foundEvidenceBlock := false
	for _, item := range events {
		event := item.(map[string]any)
		if event["type"] != "run_blocked" {
			continue
		}
		payload := event["payload"].(map[string]any)
		if payload["code"] == "evidence_insufficient" &&
			payload["next_action"] == "provide the deployed retry configuration" {
			foundEvidenceBlock = true
		}
	}
	if !foundEvidenceBlock {
		t.Fatalf("run did not expose the evidence-insufficient next action: %+v", events)
	}

	fake.append(
		toolResponse("record_reasoning", `{"record":{"id":"p-invented","kind":"property","statement":"The unobserved behavior is correct","status":"passed","required":true,"evidence_ids":["ev-invented"]}}`),
		toolResponse("command", `{"program":"git","args":["diff","--check"]}`),
		toolResponse("finish", `{"summary":"Attempted to cite evidence that this run never produced."}`),
	)
	danglingTask := postData(t, httpServer.URL+"/api/v1/tasks", map[string]any{
		"repository_id": project["id"], "title": "Reject invented evidence",
		"description": "Prove that fabricated evidence references fail closed", "task_type": "review",
	})
	danglingRun := postData(t, httpServer.URL+"/api/v1/tasks/"+danglingTask["id"].(string)+"/run", map[string]any{})
	danglingBlocked := waitForRunState(t, httpServer.URL, danglingRun["run_id"].(string), "blocked")
	danglingBundle := danglingBlocked["result_bundle"].(map[string]any)
	danglingReasoning := danglingBundle["reasoning"].(map[string]any)
	danglingAssessment := danglingReasoning["assessment"].(map[string]any)
	if len(danglingAssessment["dangling_evidence"].([]any)) != 1 {
		t.Fatalf("invented evidence reference was not detected: %+v", danglingReasoning)
	}

	server.executor = gateway.NewStepExecutor(
		map[string]llm.Provider{providerRecord.ID.String(): &durableFailingProvider{}},
		gateway.ExecutorConfig{DefaultTimeout: 2 * time.Second, MaxRetries: 0},
	)
	failedTask := postData(t, httpServer.URL+"/api/v1/tasks", map[string]any{
		"repository_id": project["id"], "title": "Provider failure",
		"description": "Exercise provider failure handling", "task_type": "bug_fix",
	})
	failedAccepted := postDataWithKey(t,
		httpServer.URL+"/api/v1/tasks/"+failedTask["id"].(string)+"/run",
		map[string]any{}, "durable-provider-failure",
	)
	failedRun := waitForTerminalRun(t, httpServer.URL, failedAccepted["run_id"].(string))
	if failedRun["state"] != "blocked" {
		t.Fatalf("provider failure should block with recoverable evidence: %+v", failedRun)
	}
	failedBundle := failedRun["result_bundle"].(map[string]any)
	warnings, _ := failedBundle["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("provider failure did not preserve an actionable warning: %+v", failedBundle)
	}
	if err := audit.VerifyChain(context.Background()); err != nil {
		t.Fatalf("valid audit chain was rejected: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		UPDATE audit_log SET details=jsonb_set(details,'{tampered}','true'::jsonb,true)
		WHERE sequence=(SELECT MAX(sequence) FROM audit_log)`); err != nil {
		t.Fatal(err)
	}
	if err := audit.VerifyChain(context.Background()); err == nil {
		t.Fatal("tampered audit evidence was not detected")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE task_runs SET state='running',lease_expires_at=NOW()-INTERVAL '1 minute' WHERE id=$1`, failedAccepted["run_id"]); err != nil {
		t.Fatal(err)
	}
	diagnostics := getData(t, httpServer.URL+"/api/v1/diagnostics/runtime").(map[string]any)
	if diagnostics["stuck_query_available"] != true || len(diagnostics["stuck_runs"].([]any)) == 0 {
		t.Fatalf("missing expired-lease signal: %+v", diagnostics)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE task_runs SET state='blocked' WHERE id=$1`, failedAccepted["run_id"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE providers SET is_active=false`); err != nil {
		t.Fatal(err)
	}
	if ready := server.runtimeReadiness(context.Background()); ready.Ready || ready.Reason != "no_active_provider" {
		t.Fatalf("missing provider reported ready: %+v", ready)
	}
}

func toolResponse(name, arguments string) *llm.ChatResponse {
	return &llm.ChatResponse{
		Model: "fake-model", InputTokens: 10, OutputTokens: 5,
		ToolCall: &llm.ToolCall{Name: name, Arguments: json.RawMessage(arguments)},
	}
}

func writeFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func gitFixture(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func postData(t *testing.T, url string, body any) map[string]any {
	return postDataWithKey(t, url, body, "")
}

func getData(t *testing.T, url string) any {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data  any `json:"data"`
		Error any `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode >= 300 {
		t.Fatalf("GET %s returned %d: %+v", url, response.StatusCode, envelope.Error)
	}
	return envelope.Data
}

func postDataWithKey(t *testing.T, url string, body any, idempotencyKey string) map[string]any {
	t.Helper()
	data, err := postDataWithKeyResult(url, body, idempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func postDataWithKeyResult(url string, body any, idempotencyKey string) (map[string]any, error) {
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var envelope struct {
		Data  map[string]any `json:"data"`
		Error any            `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s returned %d: %+v", url, response.StatusCode, envelope.Error)
	}
	return envelope.Data, nil
}

func waitForRunState(t *testing.T, baseURL, runID, wanted string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/api/v1/runs/" + runID)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		err = json.NewDecoder(response.Body).Decode(&envelope)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if state, _ := envelope.Data["state"].(string); state == wanted {
			return envelope.Data
		} else if state == "blocked" || state == "failed" || state == "cancelled" {
			t.Fatalf("run terminated in %s: %+v", state, envelope.Data["result_bundle"])
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run %s did not reach %s", runID, wanted)
	return nil
}

func waitForTerminalRun(t *testing.T, baseURL, runID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(baseURL + "/api/v1/runs/" + runID)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		err = json.NewDecoder(response.Body).Decode(&envelope)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch envelope.Data["state"] {
		case "blocked", "failed", "cancelled", "review", "completed":
			return envelope.Data
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run %s did not terminate", runID)
	return nil
}
