package api

import (
	"testing"

	semcontext "github.com/saterdoe/oberth/internal/context"
)

func TestContextOptionsForOllamaFitDefaultWindow(t *testing.T) {
	options := contextOptionsForProvider(semcontext.CompileOptions{
		MaxTokens:           8192,
		ReserveOutputTokens: 2048,
		MaxSourcesPerKind:   8,
	}, "Ollama")

	if options.MaxTokens != 8000 || options.ReserveOutputTokens != 1500 || options.MaxSourcesPerKind != 4 {
		t.Fatalf("unexpected Ollama context profile: %+v", options)
	}
}

func TestContextOptionsPreservesRemoteProviderProfile(t *testing.T) {
	want := semcontext.CompileOptions{MaxTokens: 8192, ReserveOutputTokens: 2048, MaxSourcesPerKind: 8}
	got := contextOptionsForProvider(want, "openai")
	if got.MaxTokens != want.MaxTokens || got.ReserveOutputTokens != want.ReserveOutputTokens || got.MaxSourcesPerKind != want.MaxSourcesPerKind {
		t.Fatalf("remote provider profile changed: %+v", got)
	}
}
