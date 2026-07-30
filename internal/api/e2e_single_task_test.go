//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/internal/permission"
	"github.com/saterdoe/oberth/pkg/llm"
)

func TestE2ESingleTaskFullFlow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping E2E test")
	}

	// 1. Create fixture repo
	repoDir := t.TempDir()
	initGitRepo(t, repoDir)
	err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(repoDir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`), 0644)
	require.NoError(t, err)
	gitAdd(t, repoDir, ".", "go.mod", "main.go")
	gitCommit(t, repoDir, "initial commit")

	// 2. Create the main test server
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(context.Background(), `
		DO $$
		DECLARE table_record RECORD;
		BEGIN
			FOR table_record IN
				SELECT tablename FROM pg_tables
				WHERE schemaname = 'public' AND tablename <> 'schema_migrations'
			LOOP
				EXECUTE 'TRUNCATE TABLE public.' || quote_ident(table_record.tablename) || ' CASCADE';
			END LOOP;
		END $$`)
	require.NoError(t, err)

	providerRepo := repos.NewProviderRepo(pool)
	routingRepo := repos.NewRoutingRuleRepo(pool)
	sessionRepo := repos.NewSessionRepo(pool)
	costLogRepo := repos.NewCostLogRepo(pool)
	budgetRepo := repos.NewBudgetRepo(pool)
	auditRepo := repos.NewAuditRepo(pool)
	executionRepo := repos.NewExecutionLogRepo(pool)
	approvalGateRepo := repos.NewApprovalGateRepo(pool)

	cfg := config.Default()
	cfg.Auth.Token = "e2e-local-token"
	var mockBaseURL string
	executor := gateway.NewStepExecutor(nil, gateway.ExecutorConfig{
		ResolveProvider: func(context.Context, string) (llm.Provider, error) {
			return llm.NewOpenAI(mockBaseURL, "mock-key"), nil
		},
	})
	router := gateway.NewRouter(routingRepo, providerRepo)
	srv := NewServer(pool, providerRepo, routingRepo, sessionRepo, costLogRepo, budgetRepo, auditRepo, executionRepo, approvalGateRepo, nil, nil, router, executor, nil, nil, cfg, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// 3. Start a mock LLM server that returns an edit plan
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":      "mock-completion",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "mock-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": `{"summary":"Update greeting","edits":[{"path":"` + filepath.ToSlash(repoDir) + `/main.go","operation":"replace","search":"fmt.Println(\"hello\")","replace":"fmt.Println(\"hello world\")"}]}`,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mockLLM := httptest.NewServer(mux)
	t.Cleanup(mockLLM.Close)
	mockBaseURL = mockLLM.URL

	// 4. Create provider pointing to mock LLM
	baseURL := mockLLM.URL
	providerReq := map[string]any{
		"name":          "mock-provider",
		"provider_type": "openai",
		"base_url":      baseURL,
		"api_key":       "mock-key",
		"default_model": "mock-model",
		"models":        "mock-model",
	}
	providerBody, _ := json.Marshal(providerReq)
	resp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(providerBody))
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	providerResult := decodeE2EData(t, resp)
	resp.Body.Close()
	providerID := providerResult["id"].(string)

	// 5. Create routing rule for this provider
	ruleReq := map[string]any{
		"name":        "e2e-test-rule",
		"priority":    100,
		"provider_id": providerID,
		"model":       "mock-model",
		"is_active":   true,
	}
	ruleBody, _ := json.Marshal(ruleReq)
	resp, err = http.Post(ts.URL+"/api/v1/routing-rules", "application/json", bytes.NewReader(ruleBody))
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	resp.Body.Close()

	// 6. Allow file writes via the permission engine
	srv.perm.AddRule(permission.Rule{
		Name:          "e2e-allow-writes",
		Priority:      1000,
		Operation:     "file.write",
		TargetPattern: filepath.ToSlash(repoDir) + "/*",
		Decision:      permission.Allow,
	})

	// 7. Create a project and a workspace for the fixture repo
	wsReq := map[string]any{"name": "e2e-workspace"}
	wsBody, _ := json.Marshal(wsReq)
	resp, err = http.Post(ts.URL+"/api/v1/workspaces", "application/json", bytes.NewReader(wsBody))
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	wsResult := decodeE2EData(t, resp)
	resp.Body.Close()
	workspaceID := wsResult["id"].(string)

	projectReq := map[string]any{
		"workspace_id": workspaceID,
		"name":         "e2e-project",
		"path":         repoDir,
	}
	projectBody, _ := json.Marshal(projectReq)
	resp, err = http.Post(ts.URL+"/api/v1/projects", "application/json", bytes.NewReader(projectBody))
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	projectResult := decodeE2EData(t, resp)
	resp.Body.Close()
	projectID := projectResult["id"].(string)

	// 8. Create task
	taskReq := map[string]any{
		"repository_id": projectID,
		"title":         "Update greeting",
		"description":   "Change the greeting to say hello world instead of hello",
		"task_type":     "implementation",
	}
	taskBody, _ := json.Marshal(taskReq)
	resp, err = http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewReader(taskBody))
	require.NoError(t, err)
	require.Equal(t, 201, resp.StatusCode)
	taskResult := decodeE2EData(t, resp)
	resp.Body.Close()
	taskID := taskResult["id"].(string)

	// 9. Run the task
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+taskID+"/run", "application/json", nil)
	require.NoError(t, err)
	require.Equal(t, 202, resp.StatusCode)
	resp.Body.Close()

	// 10. Poll for task completion (timeout after 30s)
	deadline := time.Now().Add(30 * time.Second)
	var finalTask map[string]any
	for time.Now().Before(deadline) {
		resp, err = http.Get(ts.URL + "/api/v1/tasks/" + taskID)
		require.NoError(t, err)
		finalTask = decodeE2EData(t, resp)
		resp.Body.Close()
		status, _ := finalTask["status"].(string)
		if status == "completed" || status == "blocked" || status == "failed" || status == "cancelled" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// 11. Assertions
	status, _ := finalTask["status"].(string)
	assert.Contains(t, []string{"completed", "blocked"}, status, "task should reach a safe terminal state; status is %s", status)

	// A completed run applies the edit. A blocked run must leave the repository
	// untouched until an approval or additional evidence is supplied.
	content, err := os.ReadFile(filepath.Join(repoDir, "main.go"))
	require.NoError(t, err)
	if status == "completed" {
		assert.Contains(t, string(content), "hello world")
	} else {
		assert.NotContains(t, string(content), "hello world")
	}
}

func decodeE2EData(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&envelope))
	require.NotNil(t, envelope.Data)
	return envelope.Data
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "--initial-branch=main")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", out)
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = dir
	cmd.Run()
}

func gitAdd(t *testing.T, dir string, args ...string) {
	t.Helper()
	allArgs := append([]string{"add"}, args...)
	cmd := exec.Command("git", allArgs...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git add failed: %s", out)
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git commit failed: %s", out)
}
