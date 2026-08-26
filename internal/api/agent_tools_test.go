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

func TestNormalizeAgentCommandSplitsAllowlistedCompleteCommand(t *testing.T) {
	program, args := normalizeAgentCommand("go test ./...", nil)
	if program != "go" || len(args) != 2 || args[0] != "test" || args[1] != "./..." {
		t.Fatalf("unexpected normalized command: %q %#v", program, args)
	}
}

func TestNormalizeAgentCommandLeavesUnknownCompleteCommandBlocked(t *testing.T) {
	program, args := normalizeAgentCommand("go run ./cmd/server", nil)
	if program != "go run ./cmd/server" || len(args) != 0 {
		t.Fatalf("unsafe command was normalized: %q %#v", program, args)
	}
}

func TestSafeAgentCommandAllowsStrictClangSyntaxCheck(t *testing.T) {
	args := []string{"-std=c11", "-Wall", "-Wextra", "-Werror", "-fsyntax-only", "main.c"}
	if !safeAgentCommand(`C:\Program Files\LLVM\bin\clang.exe`, args) {
		t.Fatal("expected strict clang syntax verification to be allowed")
	}
	if safeAgentCommand("clang", []string{"main.c", "-o", "app.exe"}) {
		t.Fatal("expected arbitrary clang output command to remain blocked")
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

func TestRecordReasoningToolDropsPlaceholderEvidenceHash(t *testing.T) {
	action := agentruntime.Action{SchemaVersion: agentruntime.SchemaVersion, Tool: "record_reasoning", Arguments: json.RawMessage(`{"evidence":{"id":"ev-1","source":"README.md","hash":"some_hash","detail":"observed"}}`)}
	observation := executeTypedTool(context.Background(), nil, "", "", "", "", "", permission.New(), nil, action)
	if observation.Status != "ok" {
		t.Fatalf("expected placeholder hash to be omitted: %+v", observation)
	}
	encoded, _ := json.Marshal(observation.Data)
	if strings.Contains(string(encoded), "some_hash") {
		t.Fatalf("placeholder hash survived normalization: %s", encoded)
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
