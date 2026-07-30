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

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/providersecret"
	"github.com/saterdoe/oberth/internal/vault"
)

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
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
	cfg.Auth.Token = "integration-test-local-token"
	vaultConn := vault.New(t.TempDir())
	require.NoError(t, vaultConn.Ensure())
	srv := NewServer(pool, providerRepo, routingRepo, sessionRepo, costLogRepo, budgetRepo, auditRepo, executionRepo, approvalGateRepo, nil, nil, nil, nil, nil, vaultConn, cfg, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestListProviders_Empty(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/providers")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	data, ok := body["data"].([]any)
	assert.True(t, ok, "response should contain a data array")
	assert.Empty(t, data)
}

func TestCreateProvider(t *testing.T) {
	ts := setupTestServer(t)

	name := "test-create-" + uuid.NewString()[:8]
	reqBody := CreateProviderRequest{
		Name:         name,
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		IsActive:     boolPtr(true),
		Priority:     intPtr(5),
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response should contain a data object")
	assert.Equal(t, name, data["name"])
	assert.Equal(t, "openai", data["provider_type"])
	assert.Equal(t, "gpt-4", data["default_model"])
	assert.NotEmpty(t, data["id"])
	assert.NotEmpty(t, data["created_at"])
	assert.NotContains(t, data, "api_key")
	assert.NotContains(t, data, "api_key_encrypted")
}

func TestCreateProviderPersistsAPIKey(t *testing.T) {
	ts := setupTestServer(t)

	name := "test-create-key-" + uuid.NewString()[:8]
	apiKey := "sk-test-create"
	reqBody := CreateProviderRequest{
		Name:         name,
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		APIKey:       &apiKey,
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data := body["data"].(map[string]any)
	providerID := data["id"].(string)
	assert.NotContains(t, data, "api_key")
	assert.NotContains(t, data, "api_key_encrypted")

	got, err := setupProviderRepo(t).GetByID(context.Background(), uuid.MustParse(providerID))
	require.NoError(t, err)
	require.NotNil(t, got.APIKeyEncrypted)
	assert.True(t, providersecret.IsSealed(*got.APIKeyEncrypted))
	assert.NotContains(t, *got.APIKeyEncrypted, apiKey)
	opened, err := providersecret.Open("integration-test-local-token", *got.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, apiKey, opened)
}

func TestCreateProviderValidation(t *testing.T) {
	ts := setupTestServer(t)

	// Missing name
	reqBody := CreateProviderRequest{
		ProviderType: "openai",
		DefaultModel: "gpt-4",
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestGetProvider(t *testing.T) {
	ts := setupTestServer(t)

	// First create a provider
	name := "test-get-" + uuid.NewString()[:8]
	createReq := CreateProviderRequest{
		Name:         name,
		ProviderType: "anthropic",
		DefaultModel: "claude-3-opus",
	}
	b, err := json.Marshal(createReq)
	require.NoError(t, err)

	createResp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(b))
	require.NoError(t, err)

	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()

	data := createBody["data"].(map[string]any)
	providerID := data["id"].(string)

	// Get by ID
	resp, err := http.Get(ts.URL + "/api/v1/providers/" + providerID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getBody map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getBody))

	getData, ok := getBody["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, name, getData["name"])
	assert.Equal(t, "anthropic", getData["provider_type"])
}

func TestGetProviderNotFound(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/providers/" + uuid.New().String())
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestUpdateProvider(t *testing.T) {
	ts := setupTestServer(t)

	// First create a provider
	name := "test-update-" + uuid.NewString()[:8]
	createReq := CreateProviderRequest{
		Name:         name,
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		Priority:     intPtr(10),
	}
	b, err := json.Marshal(createReq)
	require.NoError(t, err)

	createResp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(b))
	require.NoError(t, err)

	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()

	data := createBody["data"].(map[string]any)
	providerID := data["id"].(string)

	// Update provider
	updateReq := UpdateProviderRequest{
		Name:     strPtr("updated-" + name),
		Priority: intPtr(1),
	}
	ub, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/providers/"+providerID, bytes.NewReader(ub))
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
}

func TestUpdateProviderPersistsAPIKey(t *testing.T) {
	ts := setupTestServer(t)

	name := "test-update-key-" + uuid.NewString()[:8]
	createReq := CreateProviderRequest{
		Name:         name,
		ProviderType: "openai",
		DefaultModel: "gpt-4",
	}
	b, err := json.Marshal(createReq)
	require.NoError(t, err)

	createResp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(b))
	require.NoError(t, err)

	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()

	data := createBody["data"].(map[string]any)
	providerID := data["id"].(string)

	apiKey := "sk-test-update"
	updateReq := UpdateProviderRequest{APIKey: &apiKey}
	ub, err := json.Marshal(updateReq)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/providers/"+providerID, bytes.NewReader(ub))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	updateData := body["data"].(map[string]any)
	assert.NotContains(t, updateData, "api_key")
	assert.NotContains(t, updateData, "api_key_encrypted")

	got, err := setupProviderRepo(t).GetByID(context.Background(), uuid.MustParse(providerID))
	require.NoError(t, err)
	require.NotNil(t, got.APIKeyEncrypted)
	assert.True(t, providersecret.IsSealed(*got.APIKeyEncrypted))
	opened, err := providersecret.Open("integration-test-local-token", *got.APIKeyEncrypted)
	require.NoError(t, err)
	assert.Equal(t, apiKey, opened)
}

func TestDeleteProvider(t *testing.T) {
	ts := setupTestServer(t)

	// First create a provider
	name := "test-delete-" + uuid.NewString()[:8]
	createReq := CreateProviderRequest{
		Name:         name,
		ProviderType: "ollama",
		DefaultModel: "llama3",
	}
	b, err := json.Marshal(createReq)
	require.NoError(t, err)

	createResp, err := http.Post(ts.URL+"/api/v1/providers", "application/json", bytes.NewReader(b))
	require.NoError(t, err)

	var createBody map[string]any
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createBody))
	createResp.Body.Close()

	data := createBody["data"].(map[string]any)
	providerID := data["id"].(string)

	// Delete
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/providers/"+providerID, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify it's gone
	getResp, err := http.Get(ts.URL + "/api/v1/providers/" + providerID)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func boolPtr(b bool) *bool { return &b }

func intPtr(i int) *int { return &i }

func strPtr(s string) *string { return &s }

func setupProviderRepo(t *testing.T) *repos.ProviderRepo {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return repos.NewProviderRepo(pool)
}
