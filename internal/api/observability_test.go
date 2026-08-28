package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadinessFailsClosedAndDiagnosticsRemainAvailable(t *testing.T) {
	s := &Server{}
	response := httptest.NewRecorder()
	s.handleReadiness(response, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
	if response.Code != 503 || !strings.Contains(response.Body.String(), "database_unavailable") {
		t.Fatalf("false ready: %s", response.Body.String())
	}
	s.BeginDrain()
	if got := s.runtimeReadiness(context.Background()); got.Ready || got.Reason != "draining" {
		t.Fatalf("draining runtime ready: %+v", got)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/00000000-0000-0000-0000-000000000001/run", nil)
	request.SetPathValue("id", "00000000-0000-0000-0000-000000000001")
	response = httptest.NewRecorder()
	s.handleRunTask(response, request)
	if response.Code != 503 {
		t.Fatalf("admitted while draining: %d", response.Code)
	}
	response = httptest.NewRecorder()
	s.handleRuntimeDiagnostics(response, httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/runtime", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"stuck_query_available":false`) {
		t.Fatalf("diagnostics lost failure state: %s", response.Body.String())
	}
}
