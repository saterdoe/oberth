//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db/repos"
)

func TestGetCostSummary(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/costs")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "data")
}

func TestListCostLogs(t *testing.T) {
	ts := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/costs/logs?limit=10")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRecordCall(t *testing.T) {
	ts := setupTestServer(t)
	pool, err := pgxpool.New(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	session := &repos.Session{TaskType: "testing", Status: "active"}
	require.NoError(t, repos.NewSessionRepo(pool).Create(context.Background(), session))

	reqBody := map[string]any{
		"session_id":    session.ID.String(),
		"model":         "gpt-4",
		"tokens_input":  100,
		"tokens_output": 50,
		"cost_input":    0.002,
		"cost_output":   0.001,
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/costs/record", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestRecordCall_Validation(t *testing.T) {
	ts := setupTestServer(t)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"missing session_id", map[string]any{"model": "gpt-4"}},
		{"missing model", map[string]any{"session_id": "00000000-0000-0000-0000-000000000001"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := json.Marshal(tt.body)
			resp, err := http.Post(ts.URL+"/api/v1/costs/record", "application/json", bytes.NewReader(b))
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestCostEstimate(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{
		"task_type": "code_review",
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/costs/estimate", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSimulateCosts(t *testing.T) {
	ts := setupTestServer(t)

	reqBody := map[string]any{
		"current_monthly_spend": 100,
		"scenarios": []map[string]any{{
			"name": "balanced", "pct_local": 0.5, "pct_gpt4o": 0.5,
		}},
	}
	b, err := json.Marshal(reqBody)
	require.NoError(t, err)

	resp, err := http.Post(ts.URL+"/api/v1/costs/simulate", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
