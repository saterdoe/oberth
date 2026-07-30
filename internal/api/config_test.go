//go:build integration

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfig(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/config")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "data")
}

func TestUpdateConfig(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{"log_level": "debug"}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/config", bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestValidateConfig(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{"log_level": "debug"}
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post(ts.URL+"/api/v1/config/validate", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, data["valid"])
}

func TestUpdateConfig_InvalidJSON(t *testing.T) {
	ts := setupTestServer(t)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/config", bytes.NewReader([]byte(`not json`)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
