package agentadapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractsAreVersioned(t *testing.T) {
	if SchemaVersion == "" || ClaudeCode().Name() != "claude-code" || Codex().Name() != "codex" || OpenCode().Name() != "opencode" || Antigravity().Name() != "antigravity" {
		t.Fatal("agent adapter contract is incomplete")
	}
	if len(DefaultAgents()) < 4 {
		t.Fatal("DefaultAgents missing adapters")
	}
}

func TestResolveExecutableFromUserLocalBin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	name := "oberth-local-probe"
	path := filepath.Join(home, ".local", "bin", name+".exe")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveExecutable(name)
	if err != nil || resolved != path {
		t.Fatalf("resolved %q, err %v", resolved, err)
	}
}

func TestMissingCLIIsReportedAsCapabilityEvidenceNotTransportError(t *testing.T) {
	adapter := SupportedCLI{name: "missing", executable: "oberth-certainly-missing"}
	capabilities, err := adapter.Capabilities(t.Context())
	if err != nil {
		t.Fatalf("capability discovery must be inspectable: %v", err)
	}
	if capabilities.Installed || capabilities.Usable || capabilities.Message == "" {
		t.Fatalf("unexpected capability result: %+v", capabilities)
	}
}

func TestExecuteValidations(t *testing.T) {
	adapter := Antigravity()
	_, err := adapter.Execute(t.Context(), Request{SchemaVersion: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid schema version")
	}
	_, err = adapter.Execute(t.Context(), Request{SchemaVersion: SchemaVersion, Worktree: ""})
	if err == nil {
		t.Fatal("expected error for missing worktree")
	}

	unsupported := SupportedCLI{name: "unknown_agent", executable: "echo"}
	_, err = unsupported.Execute(t.Context(), Request{SchemaVersion: SchemaVersion, Worktree: t.TempDir(), Intention: "do work"})
	if err == nil {
		t.Fatal("expected error for unsupported agent")
	}
}

func TestCapabilitiesAndExecuteWithMockCLI(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mockCmd := filepath.Join(tempDir, "mock-agent.bat")
	scriptContent := `@echo off
if "%1"=="--version" (
  echo mock-agent version 1.0.0
  exit /b 0
)
if "%1"=="auth" (
  echo {"loggedIn": true, "authMethod": "oauth"}
  exit /b 0
)
if "%1"=="-p" (
  echo {"status": "success"}
  exit /b 0
)
if "%1"=="run" (
  echo {"status": "success"}
  exit /b 0
)
if "%1"=="exec" (
  echo {"status": "success"}
  exit /b 0
)
exit /b 0
`
	if err := os.WriteFile(mockCmd, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	// Test claude-code capabilities with auth logged in
	claudeAdapter := SupportedCLI{name: "claude-code", executable: "mock-agent"}
	caps, err := claudeAdapter.Capabilities(t.Context())
	if err != nil || !caps.Installed || !caps.Usable {
		t.Fatalf("unexpected claude capabilities: %+v, %v", caps, err)
	}

	// Test Execute for all supported CLI agents
	for _, agentName := range []string{"claude-code", "codex", "opencode", "antigravity"} {
		adapter := SupportedCLI{name: agentName, executable: "mock-agent"}
		out, err := adapter.Execute(t.Context(), Request{
			SchemaVersion: SchemaVersion,
			Worktree:      tempDir,
			Intention:     "test task",
			Context:       json.RawMessage(`{"repo":"test"}`),
		})
		if err != nil {
			t.Fatalf("execute failed for %s: %v", agentName, err)
		}
		if len(out) == 0 {
			t.Fatalf("empty output for %s", agentName)
		}
	}
}

func TestCapabilitiesClaudeAuthLoggedOut(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mockCmd := filepath.Join(tempDir, "mock-agent-loggedout.bat")
	scriptContent := `@echo off
if "%1"=="--version" (
  echo mock-agent 1.0
  exit /b 0
)
if "%1"=="auth" (
  echo {"loggedIn": false}
  exit /b 0
)
exit /b 0
`
	if err := os.WriteFile(mockCmd, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	adapter := SupportedCLI{name: "claude-code", executable: "mock-agent-loggedout"}
	caps, err := adapter.Capabilities(t.Context())
	if err != nil || caps.Usable || !strings.Contains(caps.Message, "autenticación") {
		t.Fatalf("expected logged out claude-code to not be usable: %+v", caps)
	}
}

func TestCapabilitiesVersionProbeFailed(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PATH", tempDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	mockCmd := filepath.Join(tempDir, "mock-agent-failing.bat")
	scriptContent := `@echo off
if "%1"=="--version" (
  exit /b 1
)
exit /b 1
`
	if err := os.WriteFile(mockCmd, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	adapter := SupportedCLI{name: "antigravity", executable: "mock-agent-failing"}
	caps, err := adapter.Capabilities(t.Context())
	if err != nil || !caps.Installed || caps.Usable || !strings.Contains(caps.Message, "probe de versión falló") {
		t.Fatalf("expected version probe error: %+v, %v", caps, err)
	}
}


