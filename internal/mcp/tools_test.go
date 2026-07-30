package mcp

import (
	"testing"

	"github.com/saterdoe/oberth/internal/vault"
)

func TestVaultToolsExposePortableReadContracts(t *testing.T) {
	tools := NewVaultTools(vault.New(t.TempDir()), nil)
	want := map[string]bool{"read-note": true, "search-vault": true, "get-context": true, "get-memory-index": true}
	if len(tools) != len(want) {
		t.Fatalf("got %d tools, want %d", len(tools), len(want))
	}
	for _, tool := range tools {
		if !want[tool.Name] || len(tool.InputSchema) == 0 || tool.Handler == nil {
			t.Fatalf("invalid portable tool contract: %#v", tool)
		}
	}
}
