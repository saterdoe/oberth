package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/internal/permission"
	workspacepkg "github.com/saterdoe/oberth/internal/workspace"
)

func TestSafeAgentCommandAllowsScopedNPMBuildScripts(t *testing.T) {
	for _, script := range []string{"build:dashboard", "build:pwa", "build:web:production"} {
		if !safeAgentCommand("npm", []string{"run", script}) {
			t.Errorf("expected npm run %s to be allowed", script)
		}
	}
}

func TestSafeAgentCommandRejectsUnsafeNPMBuildArguments(t *testing.T) {
	cases := [][]string{
		{"run", "dev"},
		{"run", "build:dashboard", "--", "--watch"},
		{"run", "build:dashboard;whoami"},
		{"exec", "vite"},
	}
	for _, args := range cases {
		if safeAgentCommand("npm", args) {
			t.Errorf("expected npm %v to be rejected", args)
		}
	}
}

func TestSafeAgentCommandAllowsReadOnlyGitDiffCheck(t *testing.T) {
	if !safeAgentCommand("git", []string{"diff", "--check"}) {
		t.Fatal("git diff --check must be allowed as a verification command")
	}
}

func TestRecordReasoningToolValidatesWithoutWorkspaceEffects(t *testing.T) {
	action := agentruntime.Action{
		SchemaVersion: agentruntime.SchemaVersion,
		Tool:          "record_reasoning",
		Arguments: json.RawMessage(`{
			"record": {
				"id": "u1",
				"kind": "unknown",
				"statement": "The production retry policy is unavailable",
				"status": "unresolved",
				"next_action": "inspect the deployed configuration"
			}
		}`),
	}
	observation := executeTypedTool(context.Background(), nil, "", "", "", "", "", permission.New(), nil, action)
	if observation.Status != "ok" {
		t.Fatalf("expected reasoning record to be accepted: %+v", observation)
	}
	encoded, _ := json.Marshal(observation.Data)
	if !json.Valid(encoded) {
		t.Fatalf("reasoning observation is not JSON: %s", encoded)
	}
}

func TestRecordReasoningToolNormalizesLegacyNativeArguments(t *testing.T) {
	action := agentruntime.Action{
		SchemaVersion: agentruntime.SchemaVersion,
		Tool:          "record_reasoning",
		Arguments:     json.RawMessage(`{"claim":"The healthcheck returns status ok.","evidence_id":"ev-turn-001"}`),
	}
	observation := executeTypedTool(context.Background(), nil, "", "", "", "", "", permission.New(), nil, action)
	if observation.Status != "ok" {
		t.Fatalf("expected legacy reasoning arguments to be normalized: %+v", observation)
	}
	encoded, _ := json.Marshal(observation.Data)
	if !strings.Contains(string(encoded), `"statement":"The healthcheck returns status ok."`) ||
		!strings.Contains(string(encoded), `"evidence_ids":["ev-turn-001"]`) {
		t.Fatalf("unexpected normalized reasoning observation: %s", encoded)
	}
}

func TestRecordReasoningToolNormalizesFlatNativeArguments(t *testing.T) {
	action := agentruntime.Action{
		SchemaVersion: agentruntime.SchemaVersion,
		Tool:          "record_reasoning",
		Arguments:     json.RawMessage(`{"id":"unknown-file","kind":"unknown","scope":"workspace","status":"unresolved","statement":"The requested file was not found.","confidence":0.8,"evidence_ids":[]}`),
	}
	observation := executeTypedTool(context.Background(), nil, "", "", "", "", "", permission.New(), nil, action)
	if observation.Status != "ok" {
		t.Fatalf("expected flat native reasoning arguments to be normalized: %+v", observation)
	}
	encoded, _ := json.Marshal(observation.Data)
	if !strings.Contains(string(encoded), `"id":"unknown-file"`) ||
		!strings.Contains(string(encoded), `"statement":"The requested file was not found."`) {
		t.Fatalf("unexpected normalized reasoning observation: %s", encoded)
	}
}

func TestPatchToolInfersCreateOnlyForMissingFile(t *testing.T) {
	root := t.TempDir()
	policy := permission.New()
	policy.AddRule(permission.Rule{Name: "allow test write", Priority: 1, Operation: "file.write", TargetPattern: "**", Decision: permission.Allow, IsActive: true})
	workspace, err := workspacepkg.NewRuntime("test-run", root, policy)
	if err != nil {
		t.Fatal(err)
	}
	action := agentruntime.Action{SchemaVersion: agentruntime.SchemaVersion, Tool: "patch", Arguments: json.RawMessage(`{"path":"docs/check.md","new_text":"verified\n"}`)}
	observation := executeTypedTool(context.Background(), workspace, "run", "task", "session", "implementation", "low", policy, nil, action)
	if observation.Status != "ok" {
		t.Fatalf("expected safe create inference: %+v", observation)
	}
	content, err := os.ReadFile(filepath.Join(root, "docs", "check.md"))
	if err != nil || string(content) != "verified\n" {
		t.Fatalf("unexpected created file: %q %v", content, err)
	}
}
