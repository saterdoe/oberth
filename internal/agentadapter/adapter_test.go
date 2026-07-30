package agentadapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContractsAreVersioned(t *testing.T) {
	if SchemaVersion == "" || ClaudeCode().Name() != "claude-code" || Codex().Name() != "codex" || OpenCode().Name() != "opencode" {
		t.Fatal("agent adapter contract is incomplete")
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
