package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/saterdoe/oberth/internal/permission"
)

func TestRuntimeWorkspaceContractAndRollback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	policy := permission.New()
	policy.AddRule(permission.Rule{Name: "tests", Operation: "command.exec", TargetPattern: "*", Decision: permission.Allow})
	workspace, err := NewRuntime("run-1", root, policy)
	if err != nil {
		t.Fatal(err)
	}
	set, err := workspace.ApplyPatch(context.Background(), Patch{Path: "main.go", OldText: "old", NewText: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Rollback(context.Background(), set.ID); err != nil {
		t.Fatal(err)
	}
	content, err := workspace.Read(context.Background(), "main.go")
	if err != nil || string(content) != "package old\n" {
		t.Fatalf("rollback failed: %q (%v)", content, err)
	}
}

func TestRuntimeWorkspaceCreatesAndRollsBackNewFile(t *testing.T) {
	workspace, err := NewRuntime("run-2", t.TempDir(), permission.New())
	if err != nil {
		t.Fatal(err)
	}
	set, err := workspace.ApplyPatch(context.Background(), Patch{Path: "new.go", Operation: "create", NewText: "package demo\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Rollback(context.Background(), set.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Read(context.Background(), "new.go"); !os.IsNotExist(err) {
		t.Fatalf("created file survived rollback: %v", err)
	}
}
