package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db/repos"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func createTestProviderForRouting(t *testing.T, pool *pgxpool.Pool) *repos.Provider {
	t.Helper()
	repo := repos.NewProviderRepo(pool)
	p := &repos.Provider{
		Name:         "routing-test-provider-" + uuid.NewString()[:8],
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
		Priority:     0,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	return p
}

func setupRoutingTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	pool := setupTestDB(t)

	providerRepo := repos.NewProviderRepo(pool)
	routingRepo := repos.NewRoutingRuleRepo(pool)
	sessionRepo := repos.NewSessionRepo(pool)
	costLogRepo := repos.NewCostLogRepo(pool)
	budgetRepo := repos.NewBudgetRepo(pool)
	auditRepo := repos.NewAuditRepo(pool)
	executionRepo := repos.NewExecutionLogRepo(pool)
	approvalGateRepo := repos.NewApprovalGateRepo(pool)

	srv := NewServer(pool, providerRepo, routingRepo, sessionRepo, costLogRepo, budgetRepo, auditRepo, executionRepo, approvalGateRepo, nil, nil, nil, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestListRoutingRules_Empty(t *testing.T) {
	ts := setupRoutingTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/routing-rules")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	data, ok := body["data"].([]any)
	assert.True(t, ok, "response should contain a data array")
	assert.Empty(t, data)
}

func TestCreateRoutingRule(t *testing.T) {
	ts := setupRoutingTestServer(t)
	pool := setupTestDB(t)
	provider := createTestProviderForRouting(t, pool)

	name := "test-create-rule-" + uuid.NewString()[:8]
	reqBody := CreateRoutingRuleRequest{
		Name:       name,
		Priority:   5,
		IsActive:   boolPtr(true),
		ProviderID: provider.ID.String(),
		Model:      "gpt-4",
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/routing-rules", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response should contain a data object")
	assert.Equal(t, name, data["name"])
	assert.Equal(t, float64(5), data["priority"])
	assert.NotEmpty(t, data["id"])
	assert.NotEmpty(t, data["created_at"])
}

func TestCreateRoutingRuleValidation(t *testing.T) {
	ts := setupRoutingTestServer(t)

	reqBody := CreateRoutingRuleRequest{
		Priority:   5,
		ProviderID: uuid.New().String(),
		Model:      "gpt-4",
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/routing-rules", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestCreateRoutingRule_InvalidProviderID(t *testing.T) {
	ts := setupRoutingTestServer(t)

	reqBody := CreateRoutingRuleRequest{
		Name:       "invalid-provider",
		Priority:   1,
		ProviderID: "not-a-uuid",
		Model:      "gpt-4",
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/routing-rules", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetRoutingRule(t *testing.T) {
	ts := setupRoutingTestServer(t)
	pool := setupTestDB(t)
	provider := createTestProviderForRouting(t, pool)

	// First create
	name := "test-get-rule-" + uuid.NewString()[:8]
	createReq := CreateRoutingRuleRequest{
		Name:       name,
		Priority:   3,
		ProviderID: provider.ID.String(),
		Model:      "claude-3",
	}
	b, err := json.Marshal(createReq)
	require.NoError(t, err)

	createResp, err := http.Post(ts.URL+"/api/v1/routing-rules", "application/json", bytes.NewReader(b))
	require.NoError(t, err)

	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()

	data := createBody["data"].(map[string]any)
	ruleID := data["id"].(string)

	// Get by ID
	resp, err := http.Get(ts.URL + "/api/v1/routing-rules/" + ruleID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getBody))

	getData, ok := getBody["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, name, getData["name"])
	assert.Equal(t, "claude-3", getData["model"])
}

func TestGetRoutingRuleNotFound(t *testing.T) {
	ts := setupRoutingTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/routing-rules/" + uuid.New().String())
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestUpdateRoutingRule(t *testing.T) {
	ts := setupRoutingTestServer(t)
	pool := setupTestDB(t)
	provider := createTestProviderForRouting(t, pool)

	// First create
	name := "test-update-rule-" + uuid.NewString()[:8]
	createReq := CreateRoutingRuleRequest{
		Name:       name,
		Priority:   10,
		ProviderID: provider.ID.String(),
		Model:      "gpt-4",
	}
	b, err := json.Marshal(createReq)
	require.NoError(t, err)

	createResp, err := http.Post(ts.URL+"/api/v1/routing-rules", "application/json", bytes.NewReader(b))
	require.NoError(t, err)

	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()

	data := createBody["data"].(map[string]any)
	ruleID := data["id"].(string)

	// Update
	updateReq := UpdateRoutingRuleRequest{
		Name:     strPtr("updated-" + name),
		Priority: intPtr(1),
		Model:    strPtr("claude-3"),
	}
	ub, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/routing-rules/"+ruleID, bytes.NewReader(ub))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updateBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updateBody))

	updateData, ok := updateBody["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "updated-"+name, updateData["name"])
	assert.Equal(t, float64(1), updateData["priority"])
	assert.Equal(t, "claude-3", updateData["model"])
}

func TestDeleteRoutingRule(t *testing.T) {
	ts := setupRoutingTestServer(t)
	pool := setupTestDB(t)
	provider := createTestProviderForRouting(t, pool)

	// First create
	name := "test-delete-rule-" + uuid.NewString()[:8]
	createReq := CreateRoutingRuleRequest{
		Name:       name,
		Priority:   1,
		ProviderID: provider.ID.String(),
		Model:      "gpt-4",
	}
	b, err := json.Marshal(createReq)
	require.NoError(t, err)

	createResp, err := http.Post(ts.URL+"/api/v1/routing-rules", "application/json", bytes.NewReader(b))
	require.NoError(t, err)

	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()

	data := createBody["data"].(map[string]any)
	ruleID := data["id"].(string)

	// Delete
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/routing-rules/"+ruleID, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify it's gone
	getResp, err := http.Get(ts.URL + "/api/v1/routing-rules/" + ruleID)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

func TestReorderRoutingRules(t *testing.T) {
	ts := setupRoutingTestServer(t)
	pool := setupTestDB(t)
	provider := createTestProviderForRouting(t, pool)
	rrepo := repos.NewRoutingRuleRepo(pool)
	ctx := context.Background()

	// Create 3 rules
	rules := make([]repos.RoutingRule, 3)
	for i := range rules {
		rule := &repos.RoutingRule{
			Name:       "reorder-e2e-" + uuid.NewString()[:8],
			Priority:   (i + 1) * 10,
			IsActive:   true,
			ProviderID: provider.ID,
			Model:      "gpt-4",
		}
		require.NoError(t, rrepo.Create(ctx, rule))
		rules[i] = *rule
	}

	// Reorder: reverse the order
	reorderReq := ReorderRequest{
		RuleIDs: []string{rules[2].ID.String(), rules[1].ID.String(), rules[0].ID.String()},
	}
	b, err := json.Marshal(reorderReq)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/routing-rules/reorder", bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify order
	got, err := rrepo.GetByID(ctx, rules[2].ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Priority)

	got, err = rrepo.GetByID(ctx, rules[1].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.Priority)

	got, err = rrepo.GetByID(ctx, rules[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.Priority)
}

func TestReorderRoutingRules_EmptyIDs(t *testing.T) {
	ts := setupRoutingTestServer(t)

	reorderReq := ReorderRequest{
		RuleIDs: []string{},
	}
	b, err := json.Marshal(reorderReq)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/routing-rules/reorder", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
