package doctor

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDaemonHealthContract(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	t.Setenv("OBERTH_DOCTOR_DAEMON_URL", server.URL)
	if err := daemonHealth(); err != nil {
		t.Fatalf("expected healthy daemon: %v", err)
	}
}

func TestFetchDaemonStatusRequiresHealthyData(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"database":{"state":"healthy"},"providers":{"active":1,"health":[{"active":true,"state":"healthy"}]}}}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	t.Setenv("OBERTH_DOCTOR_STATUS_URL", server.URL)
	t.Setenv("OBERTH_AUTH_TOKEN", "test-token")
	status, err := fetchDaemonStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Database.State != "healthy" || status.Providers.Active != 1 {
		t.Fatalf("unexpected status: %+v", status)
	}
}
