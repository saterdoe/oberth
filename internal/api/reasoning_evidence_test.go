package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/internal/reasoning"
)

func TestAttachAutomaticEvidenceMakesReadCitableAndRevalidatable(t *testing.T) {
	action := agentruntime.Action{
		Tool: "read", Arguments: json.RawMessage(`{"path":"main.go"}`),
	}
	observation := &agentruntime.Observation{
		Tool: "read", Status: "ok",
		Data: map[string]any{"path": "main.go", "content": "package main\n"},
	}
	attachAutomaticEvidence(2, action, observation)
	if observation.Evidence == nil {
		t.Fatal("read observation did not receive evidence")
	}
	if observation.Evidence.ID != "ev-turn-002" ||
		observation.Evidence.Source != "file:main.go" ||
		observation.Evidence.Subject != "file:main.go" ||
		observation.Evidence.SubjectHash == "" {
		t.Fatalf("unexpected read evidence: %+v", observation.Evidence)
	}
}

func TestAttachAutomaticEvidenceBindsVerificationToDiff(t *testing.T) {
	action := agentruntime.Action{
		Tool: "command", Arguments: json.RawMessage(`{"program":"go","args":["test","./..."]}`),
	}
	observation := &agentruntime.Observation{
		Tool: "command", Status: "ok",
		Data: map[string]any{"command": "go test ./...", "result": map[string]any{"status": "ok"}},
	}
	attachAutomaticEvidence(4, action, observation)
	if observation.Evidence == nil || observation.Evidence.Subject != "diff" ||
		observation.Evidence.Source != "command:go test ./..." {
		t.Fatalf("unexpected command evidence: %+v", observation.Evidence)
	}
	if observation.Evidence.SubjectHash != "" {
		t.Fatal("diff binding must be filled from the finalized candidate diff")
	}
}

func TestAttachAutomaticEvidenceIgnoresMutationActions(t *testing.T) {
	observation := &agentruntime.Observation{Tool: "patch", Status: "ok", Data: map[string]any{"path": "main.go"}}
	attachAutomaticEvidence(1, agentruntime.Action{Tool: "patch"}, observation)
	if observation.Evidence != nil {
		t.Fatalf("patch action must not be treated as evidence: %+v", observation.Evidence)
	}
}

func TestRefreshReasoningEvidenceMarksChangedFilesStale(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	action := agentruntime.Action{Tool: "read", Arguments: json.RawMessage(`{"path":"main.go"}`)}
	observation := &agentruntime.Observation{
		Tool: "read", Status: "ok", Data: map[string]any{"path": "main.go", "content": "before"},
	}
	attachAutomaticEvidence(1, action, observation)
	current := &reasoning.CaseV1{
		Records: []reasoning.Record{{
			ID: "f1", Kind: reasoning.KindFact, Statement: "file says before",
			Status: reasoning.StatusSupported, Required: true, EvidenceIDs: []string{"ev-turn-001"},
		}},
		Evidence: []reasoning.EvidenceRef{{
			ID: observation.Evidence.ID, Source: observation.Evidence.Source,
			Hash: observation.Evidence.Hash, Subject: observation.Evidence.Subject,
			SubjectHash: observation.Evidence.SubjectHash,
		}},
	}
	if err := os.WriteFile(path, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshReasoningEvidence(root, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", current)
	if !current.Evidence[0].Stale || len(current.Assessment.GateBlockers) != 1 {
		t.Fatalf("stale required evidence must block: %+v", current)
	}
}
