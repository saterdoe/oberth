package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestCorrectRecordsOutcomeThenStartsRetry(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/runs/run-1":
			_, _ = w.Write([]byte(`{"data":{"id":"run-1","task_id":"task-1","state":"review","result_bundle":{}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/runs/run-1/outcome":
			_, _ = w.Write([]byte(`{"data":{"outcome":"corrected"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tasks/task-1/run":
			_, _ = w.Write([]byte(`{"data":{"run_id":"run-2","task_id":"task-1","session_id":"session-2","status":"running"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	parts := strings.Split(server.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	previousPort := apiPort
	apiPort = port
	defer func() { apiPort = previousPort }()

	if err := correctCmd.RunE(correctCmd, []string{"run-1"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /api/v1/runs/run-1",
		"POST /api/v1/runs/run-1/outcome",
		"POST /api/v1/tasks/task-1/run",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected calls: %v", calls)
	}
}
