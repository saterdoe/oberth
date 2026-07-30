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
