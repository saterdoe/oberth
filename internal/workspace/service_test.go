package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/saterdoe/oberth/internal/permission"
)

func TestServiceRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Read(context.Background(), filepath.Join(root, "file.txt")); !errors.Is(err, ErrAbsolutePath) {
		t.Fatalf("expected absolute path rejection, got %v", err)
	}
	if err := service.WriteExisting(context.Background(), "../outside.txt", []byte("no")); !errors.Is(err, permission.ErrOutsideWorkspace) {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestServiceWritesExistingFileAtomically(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.WriteExisting(context.Background(), "main.go", []byte("after")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "after" {
		t.Fatalf("unexpected content %q", data)
	}
}

func TestServiceCreatesMissingParentDirectoriesInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Create(context.Background(), "docs/qa/result.md", []byte("verified\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "qa", "result.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "verified\n" {
		t.Fatalf("unexpected content %q", data)
	}
}

func TestApplyRejectsConflictWithoutPartialWrites(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("before"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	service, _ := New(root)
	_, err := service.Apply(context.Background(), "change-1", []Change{
		{Path: "a.txt", Content: []byte("after")},
		{Path: "b.txt", Content: []byte("after"), ExpectedHash: "wrong"},
	})
	if err == nil {
		t.Fatal("expected conflict")
	}
	data, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(data) != "before" {
		t.Fatalf("partial write occurred: %q", data)
	}
}
