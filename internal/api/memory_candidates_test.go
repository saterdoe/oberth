package api

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/reasoning"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
)

func TestReasoningMemoryProposalsKeepOnlyRevalidatableKnowledge(t *testing.T) {
	confidence := .95
	current := &reasoning.CaseV1{
		Records: []reasoning.Record{
			{ID: "f1", Kind: reasoning.KindFact, Statement: "The API uses typed actions", Status: reasoning.StatusSupported, Confidence: &confidence, EvidenceIDs: []string{"e1"}},
			{ID: "p1", Kind: reasoning.KindProperty, Statement: "The suite passes", Status: reasoning.StatusPassed, EvidenceIDs: []string{"e2"}},
			{ID: "u1", Kind: reasoning.KindUnknown, Statement: "Production config unavailable", Status: reasoning.StatusUnresolved},
			{ID: "h1", Kind: reasoning.KindHypothesis, Statement: "Maybe faster", Status: reasoning.StatusOpen},
		},
		Experiments: []reasoning.Experiment{
			{ID: "x1", Question: "Does it pass?", Command: "go test ./...", Observation: "passed", Status: reasoning.StatusPassed, EvidenceIDs: []string{"e2"}},
			{ID: "x2", Question: "Does it scale?", Command: "bench", Observation: "unknown", Status: reasoning.StatusUnknown, EvidenceIDs: []string{"e3"}},
		},
	}
	got := reasoningMemoryProposals(current)
	if len(got) != 3 {
		t.Fatalf("expected fact, property and passed experiment, got %+v", got)
	}
	if got[0].ClaimID != "f1" || got[0].Confidence != confidence || !reflect.DeepEqual(got[0].EvidenceIDs, []string{"e1"}) {
		t.Fatalf("fact provenance was not preserved: %+v", got[0])
	}
	if got[2].Kind != "experiment" || got[2].ClaimID != "x1" {
		t.Fatalf("experiment was not converted to revalidatable memory: %+v", got[2])
	}
}

func TestVerifiedRunMemoryProposalFallsBackToCurrentCommandEvidence(t *testing.T) {
	runID := uuid.New()
	current := &reasoning.CaseV1{Evidence: []reasoning.EvidenceRef{
		{ID: "e-read", Source: "file:README.md"},
		{ID: "e-stale", Source: "command:go test ./...", Stale: true},
		{ID: "e-command", Source: "command:git diff --check"},
	}}
	proposal := verifiedRunMemoryProposal(runID, "Add live check", current, []gitpkg.DiffFile{{Path: "LIVE_CHECK.md"}})
	if proposal == nil || proposal.ClaimID != "verified-run:"+runID.String() ||
		!reflect.DeepEqual(proposal.EvidenceIDs, []string{"e-command"}) ||
		!strings.Contains(proposal.Content, "LIVE_CHECK.md") {
		t.Fatalf("unexpected verified fallback memory: %+v", proposal)
	}
}

func TestMemoryContradictionRequiresSharedSubjectAndOppositePolarity(t *testing.T) {
	if !memoryContradicts("Retries are idempotent for payment requests", "Retries are not idempotent for payment requests") {
		t.Fatal("expected opposite claims about the same subject to contradict")
	}
	if memoryContradicts("Retries are idempotent", "Caching is not enabled") {
		t.Fatal("different subjects must not be treated as contradictions")
	}
}
