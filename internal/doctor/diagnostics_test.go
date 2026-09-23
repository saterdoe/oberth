package doctor

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleOmitsConfigAndKeepsRedactedJSONValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundle.zip")
	err := CreateBundle(path, BundleInput{Config: "arbitrary_credential: never-export-me", Logs: "-----BEGIN PRIVATE KEY-----\nprivate-material\n-----END PRIVATE KEY-----", Errors: []string{`provider: password="secret with spaces"`}})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"never-export-me", "private-material", "secret with spaces"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("secret in %s", file.Name)
			}
		}
		if strings.HasSuffix(file.Name, ".json") && !json.Valid(data) {
			t.Fatalf("invalid JSON in %s: %s", file.Name, data)
		}
	}
	if err := CreateBundle(path, BundleInput{}); err == nil {
		t.Fatal("overwrote existing bundle")
	}
}

func TestFetchRuntimeDiagnosticsAuthenticated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/diagnostics/runtime" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Errorf("wrong request: %s", r.URL.Path)
			w.WriteHeader(401)
			return
		}
		io.WriteString(w, `{"data":{"schema_version":"1","readiness":{"ready":false,"reason":"draining"},"prompt":"must not be retained"}}`)
	}))
	defer server.Close()
	t.Setenv("OBERTH_DOCTOR_STATUS_URL", server.URL+"/api/v1/status")
	t.Setenv("OBERTH_AUTH_TOKEN", "fixture")
	result, err := FetchRuntimeDiagnostics()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	if result.Readiness.Reason != "draining" || strings.Contains(string(data), "must not be retained") {
		t.Fatalf("bad diagnostics: %s", data)
	}
}
