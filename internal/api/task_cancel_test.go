package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestCancelTaskSignalsActiveRunAndRemovesRegistration(t *testing.T) {
	taskID := uuid.New()
	cancelled := make(chan struct{})
	server := &Server{
		activeRuns: map[uuid.UUID]context.CancelFunc{
			taskID: func() { close(cancelled) },
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID.String()+"/cancel", nil)
	request.SetPathValue("id", taskID.String())
	response := httptest.NewRecorder()

	server.handleCancelTask(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("got %d, want %d: %s", response.Code, http.StatusAccepted, response.Body.String())
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("active run context was not cancelled")
	}
	if _, exists := server.activeRuns[taskID]; exists {
		t.Fatal("cancelled task remained registered as active")
	}
}

func TestCancelTaskRejectsInvalidIDWithoutTouchingRuns(t *testing.T) {
	server := &Server{activeRuns: map[uuid.UUID]context.CancelFunc{}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/not-a-uuid/cancel", nil)
	request.SetPathValue("id", "not-a-uuid")
	response := httptest.NewRecorder()

	server.handleCancelTask(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}
