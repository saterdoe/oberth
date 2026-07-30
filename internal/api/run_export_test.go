package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saterdoe/oberth/internal/reasoning"
)

func TestRenderResultBundleMarkdownIsReviewable(t *testing.T) {
	bundle := json.RawMessage(`{
		"schema_version":"1",
		"base_commit":"abc123",
		"branch":"oberth/session",
		"diff":[{"path":"main.go","status":"modified","content":"-old\n+new"}],
		"diff_hash":"sha256:diff",
		"context_hash":"sha256:context",
		"cost":0.0123,
		"tokens_input":100,
		"tokens_output":20,
		"verification_status":"passed",
		"outcome":"accepted",
		"environment":{"os":"windows","arch":"amd64","go_version":"go1.24"}
	}`)
	output := renderResultBundleMarkdown("run-1", "review", bundle)
	for _, expected := range []string{
		"# oberth run run-1", "Verification: **passed**", "Cost: **$0.0123**",
		"`main.go` (modified)", "````diff", "100 input / 20 output",
		"Human outcome: **accepted**",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in export:\n%s", expected, output)
		}
	}
}

func TestRenderResultBundleMarkdownIncludesReviewableReasoning(t *testing.T) {
	bundle, err := json.Marshal(map[string]any{
		"schema_version": "1",
		"reasoning": reasoning.CaseV1{
			SchemaVersion: reasoning.SchemaVersion,
			Records: []reasoning.Record{
				{
					ID: "h1", Kind: reasoning.KindHypothesis,
					Statement: "The parser drops empty values", Status: reasoning.StatusSupported,
					EvidenceIDs: []string{"e1"}, Falsifier: "An empty value survives parsing",
				},
				{
					ID: "u1", Kind: reasoning.KindUnknown,
					Statement: "Production configuration was not available", Status: reasoning.StatusUnresolved,
					NextAction: "inspect the deployed configuration",
				},
			},
			Evidence: []reasoning.EvidenceRef{{
				ID: "e1", Source: "command:go test ./parser",
				Hash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			}},
			Assessment: reasoning.Assessment{
				MaterialRecords: 2, SupportedRecords: 1, CoveragePercent: 50,
				MissingEvidence: []string{"u1"}, DanglingEvidence: []string{},
				GateBlockers: []string{},
			},
			Experiments: []reasoning.Experiment{{
				ID: "x1", Question: "Does the parser pass?", Environment: "windows/amd64",
				Command: "go test ./parser", Expectation: "pass", Observation: "passed",
				Status: reasoning.StatusPassed, DurationMS: 1250, Cost: 0.001,
				EvidenceIDs: []string{"e1"}, ClaimIDs: []string{"h1"},
				Baseline: "sha256:base", Candidate: "sha256:candidate",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderResultBundleMarkdown("run-1", "review", bundle)
	for _, expected := range []string{
		"## Reasoning evidence",
		"**hypothesis · supported**",
		"evidence: `e1`",
		"Falsifier: An empty value survives parsing",
		"Next action: inspect the deployed configuration",
		"`e1`: command:go test ./parser",
		"Coverage: **50%**",
		"### Reproducible experiments",
		"**x1 · passed**",
		"Comparison: `sha256:base` → `sha256:candidate`",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("missing %q in:\n%s", expected, rendered)
		}
	}
}
