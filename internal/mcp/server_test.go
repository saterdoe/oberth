package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/saterdoe/oberth/internal/vault"
)

func TestMCPReadOnlyMemorySurface(t *testing.T) {
	root := t.TempDir()
	v := vault.New(root)
	if _, err := v.CreateNote("memory-index", "stable fact", nil); err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	for _, tool := range NewVaultTools(v, nil) {
		if tool.Name == "create-note" || tool.Name == "update-note" || tool.Name == "compact-session" {
			t.Fatalf("canonical memory mutation must not be exposed over MCP: %s", tool.Name)
		}
		server.RegisterTool(tool)
	}
	var output bytes.Buffer
	err := server.HandleMessage(context.Background(), bytes.NewBufferString(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get-memory-index","arguments":{}}}`,
	), &output)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(output.Bytes(), &response); err != nil || response["result"] == nil {
		t.Fatalf("invalid MCP response: %s (%v)", output.String(), err)
	}
}

func TestMCPRejectsUnknownTool(t *testing.T) {
	var output bytes.Buffer
	err := NewServer().HandleMessage(context.Background(), bytes.NewBufferString(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write-file","arguments":{}}}`,
	), &output)
	if err == nil {
		t.Fatal("unknown tools must be rejected")
	}
}
