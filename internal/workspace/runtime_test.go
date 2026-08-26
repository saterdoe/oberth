package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestRuntimeWorkspaceReplaceNormalizesModelLineEndings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	original := "package main\r\n\r\nfunc Greeting() string {\r\n\treturn \"hello\"\r\n}\r\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	workspace, err := NewRuntime("run-crlf", root, permission.New())
	if err != nil {
		t.Fatal(err)
	}
	_, err = workspace.ApplyPatch(context.Background(), Patch{
		Path:    "main.go",
		OldText: "\treturn \"hello\"\n}",
		NewText: "\treturn \"Hello from Oberth\"\n}",
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); got != strings.Replace(original, "hello", "Hello from Oberth", 1) {
		t.Fatalf("line endings were not preserved: %q", got)
	}
}
