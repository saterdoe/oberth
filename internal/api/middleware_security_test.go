package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalHostOnlyRejectsDNSRebindingHost(t *testing.T) {
	called := false
	handler := LocalHostOnly(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	for _, host := range []string{
		"attacker.example",
		"localhost.attacker.example:9090",
		"127.0.0.1.attacker.example",
		"",
	} {
		request := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/health", nil)
		request.Host = host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("host %q: expected 403, got %d", host, response.Code)
		}
	}
	if called {
		t.Fatal("downstream handler was called for a denied host")
	}
}

func TestLocalHostOnlyAllowsExactLoopbackHosts(t *testing.T) {
	handler := LocalHostOnly(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, host := range []string{"localhost:9090", "127.0.0.1:9090", "[::1]:9090"} {
		request := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/health", nil)
		request.Host = host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Errorf("host %q: expected 204, got %d", host, response.Code)
		}
	}
}

func TestSecurityHeadersHardenEveryResponse(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/health", nil))

	for name, want := range map[string]string{
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
	} {
		if got := response.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}
