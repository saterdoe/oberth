package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTransactionsRecoverEveryOperationAfterRestart(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(*testing.T, string)
		operation  Operation
		assertBack func(*testing.T, string)
	}{
		{
			name: "create", operation: Operation{Kind: OperationCreate, Path: "created.txt", Content: []byte("new")},
			prepare: func(*testing.T, string) {},
			assertBack: func(t *testing.T, root string) {
				if _, err := os.Stat(filepath.Join(root, "created.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("created file survived recovery: %v", err)
				}
			},
		},
		{
			name: "replace", operation: Operation{Kind: OperationReplace, Path: "file.txt", Content: []byte("after")},
			prepare:    func(t *testing.T, root string) { writeFixture(t, root, "file.txt", "before", 0600) },
			assertBack: func(t *testing.T, root string) { assertFixture(t, root, "file.txt", "before", 0600) },
		},
		{
			name: "delete", operation: Operation{Kind: OperationDelete, Path: "file.txt"},
			prepare:    func(t *testing.T, root string) { writeFixture(t, root, "file.txt", "before", 0600) },
			assertBack: func(t *testing.T, root string) { assertFixture(t, root, "file.txt", "before", 0600) },
		},
		{
			name: "rename", operation: Operation{Kind: OperationRename, Path: "from.txt", To: "nested/to.txt"},
			prepare: func(t *testing.T, root string) { writeFixture(t, root, "from.txt", "before", 0600) },
			assertBack: func(t *testing.T, root string) {
				assertFixture(t, root, "from.txt", "before", 0600)
				if _, err := os.Stat(filepath.Join(root, "nested", "to.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("rename destination survived recovery: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			test.prepare(t, root)
			service, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			service.transactionHook = func(string) error { return errors.New("crash") }
			err = service.ApplyOperations(context.Background(), "tx-"+test.name, []Operation{test.operation})
			if !errors.Is(err, ErrInjectedCrash) {
				t.Fatalf("expected injected crash, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(service.transactionRoot(), "tx-"+test.name, "manifest.json")); err != nil {
				t.Fatalf("journal was removed before recovery: %v", err)
			}
			if _, err := New(root); err != nil {
				t.Fatalf("restart recovery: %v", err)
			}
			test.assertBack(t, root)
			if _, err := os.Stat(filepath.Join(service.transactionRoot(), "tx-"+test.name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("recovered journal was not removed: %v", err)
			}
		})
	}
}

func TestCommittedTransactionRemovesJournal(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "file.txt", "before", 0600)
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyOperations(context.Background(), "committed", []Operation{{Kind: OperationReplace, Path: "file.txt", Content: []byte("after")}}); err != nil {
		t.Fatal(err)
	}
	assertFixture(t, root, "file.txt", "after", 0600)
	if _, err := os.Stat(filepath.Join(service.transactionRoot(), "committed")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed journal remains: %v", err)
	}
}

func TestRestartRollsBackPartialMultiFileChangeSet(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "a.txt", "a-before", 0600)
	writeFixture(t, root, "b.txt", "b-before", 0600)
	service, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	service.transactionHook = func(string) error {
		calls++
		if calls == 1 {
			return errors.New("crash between files")
		}
		return nil
	}
	err = service.ApplyOperations(context.Background(), "partial", []Operation{
		{Kind: OperationReplace, Path: "a.txt", Content: []byte("a-after")},
		{Kind: OperationReplace, Path: "b.txt", Content: []byte("b-after")},
	})
	if !errors.Is(err, ErrInjectedCrash) {
		t.Fatalf("expected injected crash, got %v", err)
	}
	if _, err := New(root); err != nil {
		t.Fatal(err)
	}
	assertFixture(t, root, "a.txt", "a-before", 0600)
	assertFixture(t, root, "b.txt", "b-before", 0600)
}

func writeFixture(t *testing.T, root, name, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func assertFixture(t *testing.T, root, name, content string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil || string(data) != content {
		t.Fatalf("fixture %s = %q (%v)", name, data, err)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
	if runtime.GOOS == "windows" {
		mode = 0666
	}
	if err != nil || info.Mode().Perm() != mode {
		t.Fatalf("fixture mode %s = %v (%v)", name, info.Mode().Perm(), err)
	}
}
