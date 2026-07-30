package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownRequiresConfiguredLifecycle(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()
	server.handleShutdown(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/shutdown", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestShutdownAcknowledgesBeforeTriggeringLifecycle(t *testing.T) {
	server := &Server{}
	called := make(chan struct{}, 1)
	server.SetShutdown(func() { called <- struct{}{} })
	response := httptest.NewRecorder()

	server.handleShutdown(response, httptest.NewRequest(http.MethodPost, "/api/v1/system/shutdown", nil))

	if response.Code != http.StatusAccepted {
		t.Fatalf("got %d, want %d", response.Code, http.StatusAccepted)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not invoked")
	}
}
