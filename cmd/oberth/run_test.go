package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveGitRootFromNestedDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repository with spaces")
	nested := filepath.Join(root, "src", "feature")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "init", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	got, err := resolveGitRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) || !samePath(got, want) {
		t.Fatalf("root=%q want=%q", got, want)
	}
}

func TestResolveGitRootRejectsNonRepository(t *testing.T) {
	_, err := resolveGitRoot(t.TempDir())
	if err == nil {
		t.Fatal("expected non-repository error")
	}
}

func samePath(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}
