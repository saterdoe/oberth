//go:build integration

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListBudgets_Empty(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/budgets")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCreateBudget(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{
		"name":       "test-budget",
		"soft_limit": 50.0,
		"hard_limit": 100.0,
		"period":     "monthly",
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/budgets", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-budget", data["name"])
}

func TestCreateBudget_Validation(t *testing.T) {
	ts := setupTestServer(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"empty name", map[string]any{"soft_limit": 10, "hard_limit": 20, "period": "monthly"}},
		{"missing soft_limit", map[string]any{"name": "b", "hard_limit": 20, "period": "monthly"}},
		{"missing hard_limit", map[string]any{"name": "b", "soft_limit": 10, "period": "monthly"}},
		{"missing period", map[string]any{"name": "b", "soft_limit": 10, "hard_limit": 20}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := json.Marshal(tt.body)
			resp, err := http.Post(ts.URL+"/api/v1/budgets", "application/json", bytes.NewReader(b))
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestGetBudget(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{"name": "get-budget", "soft_limit": 10, "hard_limit": 20, "period": "weekly"}
	b, _ := json.Marshal(reqBody)
	createResp, _ := http.Post(ts.URL+"/api/v1/budgets", "application/json", bytes.NewReader(b))
	var createResult map[string]any
	json.NewDecoder(createResp.Body).Decode(&createResult)
	createResp.Body.Close()
	budgetID := createResult["data"].(map[string]any)["id"].(string)

	resp, err := http.Get(ts.URL + "/api/v1/budgets/" + budgetID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetBudget_NotFound(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/budgets/" + uuid.New().String())
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateBudget(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{"name": "update-budget", "soft_limit": 10, "hard_limit": 20, "period": "monthly"}
	b, _ := json.Marshal(reqBody)
	createResp, _ := http.Post(ts.URL+"/api/v1/budgets", "application/json", bytes.NewReader(b))
	var createResult map[string]any
	json.NewDecoder(createResp.Body).Decode(&createResult)
	createResp.Body.Close()
	budgetID := createResult["data"].(map[string]any)["id"].(string)

	updateBody := map[string]any{"soft_limit": 75.0}
	ub, _ := json.Marshal(updateBody)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/budgets/"+budgetID, bytes.NewReader(ub))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestDeleteBudget(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{"name": "delete-budget", "soft_limit": 10, "hard_limit": 20, "period": "daily"}
	b, _ := json.Marshal(reqBody)
	createResp, _ := http.Post(ts.URL+"/api/v1/budgets", "application/json", bytes.NewReader(b))
	var createResult map[string]any
	json.NewDecoder(createResp.Body).Decode(&createResult)
	createResp.Body.Close()
	budgetID := createResult["data"].(map[string]any)["id"].(string)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/budgets/"+budgetID, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
