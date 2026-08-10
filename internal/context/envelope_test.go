package context

import (
	"encoding/json"
	"testing"
)

func TestContextEnvelopeFingerprintIsDeterministicAndDetectsMutation(t *testing.T) {
	compiled := &CompileResult{
		Manifest: []SourceSelection{{ID: "internal/api/task.go:1-20", Kind: "code", Hash: "sha256:source", Reason: "matched symbol", Tokens: 42}},
		Metrics:  CompileMetrics{Selected: 1, SelectedTokens: 42},
	}
	task := ContextEnvelopeTaskV1{ID: "task-1", Title: "Add envelope", Description: "Keep stages aligned", Type: "implementation", Constraints: json.RawMessage(`{"mode":"implementation"}`)}
	repo := ContextEnvelopeRepoV1{Identity: "github.com/example/repo", BaseCommit: "abc123"}

	first, err := NewContextEnvelopeV1(task, repo, compiled, RetrievalMetrics{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewContextEnvelopeV1(task, repo, compiled, RetrievalMetrics{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == "" || first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprint is not deterministic: %q != %q", first.Fingerprint, second.Fingerprint)
	}
	if err := first.VerifyFingerprint(); err != nil {
		t.Fatalf("valid fingerprint rejected: %v", err)
	}

	first.Repository.BaseCommit = "different"
	if err := first.VerifyFingerprint(); err == nil {
		t.Fatal("expected mutated envelope fingerprint to be rejected")
	}
}

func TestContextEnvelopeRequiresStableIdentity(t *testing.T) {
	_, err := NewContextEnvelopeV1(
		ContextEnvelopeTaskV1{ID: "task-1"},
		ContextEnvelopeRepoV1{},
		&CompileResult{},
		RetrievalMetrics{},
	)
	if err == nil {
		t.Fatal("expected missing repository identity to be rejected")
	}
}
