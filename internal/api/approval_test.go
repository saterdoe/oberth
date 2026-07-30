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

func TestListApprovalGates_Empty(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/approval-gates")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCreateApprovalGate(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{
		"name":             "test-gate",
		"require_approval": true,
		"priority":         10,
		"is_active":        true,
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/approval-gates", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test-gate", data["name"])
	assert.NotEmpty(t, data["id"])
}

func TestCreateApprovalGate_Validation(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Post(ts.URL+"/api/v1/approval-gates", "application/json", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestDeleteApprovalGate(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{"name": "delete-gate", "priority": 1, "is_active": true}
	b, _ := json.Marshal(reqBody)
	createResp, _ := http.Post(ts.URL+"/api/v1/approval-gates", "application/json", bytes.NewReader(b))
	var createResult map[string]any
	json.NewDecoder(createResp.Body).Decode(&createResult)
	createResp.Body.Close()
	gateID := createResult["data"].(map[string]any)["id"].(string)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/approval-gates/"+gateID, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestDeleteApprovalGate_NotFound(t *testing.T) {
	ts := setupTestServer(t)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/approval-gates/"+uuid.New().String(), nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCheckApprovalGate_NoMatch(t *testing.T) {
	ts := setupTestServer(t)

	checkBody := map[string]string{"repo_path": "unknown", "task_type": "unknown"}
	b, _ := json.Marshal(checkBody)
	resp, err := http.Post(ts.URL+"/api/v1/approval-gates/check", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, data["requires_approval"])
}

func TestCheckApprovalGate_WithMatch(t *testing.T) {
	ts := setupTestServer(t)

	gateBody := map[string]any{
		"name":             "check-gate",
		"task_type":        "deploy",
		"require_approval": true,
		"priority":         5,
		"is_active":        true,
	}
	b, _ := json.Marshal(gateBody)
	_, _ = http.Post(ts.URL+"/api/v1/approval-gates", "application/json", bytes.NewReader(b))

	checkBody := map[string]string{"repo_path": "", "task_type": "deploy"}
	cb, _ := json.Marshal(checkBody)
	resp, err := http.Post(ts.URL+"/api/v1/approval-gates/check", "application/json", bytes.NewReader(cb))
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, data["requires_approval"])
	assert.Equal(t, "check-gate", data["gate"])
}
