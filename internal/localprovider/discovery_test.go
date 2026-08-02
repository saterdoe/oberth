package localprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoversRunningLocalInferenceServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/tags") {
			w.Write([]byte(`{"models":[{"name":"qwen-coder"}]}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"local-model"}]}`))
	}))
	defer server.Close()

	d := discoverer{
		lookPath: func(name string) (string, error) { return name, nil },
		client:   server.Client(),
		goos:     "linux",
	}
	models, running := d.ollamaModels(context.Background(), server.URL)
	if !running || len(models) != 1 || models[0] != "qwen-coder" {
		t.Fatalf("ollama models %v, running %v", models, running)
	}
	models, running = d.openAIModels(context.Background(), server.URL)
	if !running || len(models) != 1 || models[0] != "local-model" {
		t.Fatalf("openai models %v, running %v", models, running)
	}
}

func TestDiscover(t *testing.T) {
	candidates := Discover(context.Background())
	if len(candidates) < 5 {
		t.Fatalf("expected candidates list including ollama, lmstudio, and agents, got %d", len(candidates))
	}
	hasAntigravity := false
	for _, c := range candidates {
		if c.ID == "antigravity" {
			hasAntigravity = true
		}
	}
	if !hasAntigravity {
		t.Fatalf("expected antigravity candidate in Discover()")
	}
}

func TestAgentDisplayNameAndCandidateMessage(t *testing.T) {
	if agentDisplayName("antigravity") != "Antigravity" {
		t.Fatalf("expected Antigravity display name")
	}
	if agentDisplayName("unknown_id") != "unknown_id" {
		t.Fatalf("expected fallback display name")
	}

	cReady := Candidate{Usable: true}
	if candidateMessage(cReady, "help") != "Listo para usar." {
		t.Fatalf("expected ready message")
	}

	cInstalled := Candidate{Installed: true}
	if candidateMessage(cInstalled, "installed help") != "installed help" {
		t.Fatalf("expected installed help message")
	}

	cNone := Candidate{}
	if candidateMessage(cNone, "help") != "No detectado en este equipo." {
		t.Fatalf("expected not detected message")
	}
}

func TestAppInstalledChecks(t *testing.T) {
	dWin := discoverer{goos: "windows", lookPath: func(name string) (string, error) { return "", nil }}
	_ = dWin.lmStudioAppInstalled()
	_ = dWin.claudeDesktopInstalled()

	dMac := discoverer{goos: "darwin", lookPath: func(name string) (string, error) { return "", nil }}
	_ = dMac.lmStudioAppInstalled()
	_ = dMac.claudeDesktopInstalled()

	dLinux := discoverer{goos: "linux", lookPath: func(name string) (string, error) { return "", nil }}
	_ = dLinux.lmStudioAppInstalled()
	_ = dLinux.claudeDesktopInstalled()
}

