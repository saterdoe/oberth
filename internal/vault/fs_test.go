package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestVault(t *testing.T) (*Vault, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "vault-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	vaultRoot := filepath.Join(dir, ".agent-vault")
	err = os.MkdirAll(vaultRoot, 0755)
	require.NoError(t, err)
	return New(vaultRoot), vaultRoot
}

func TestCreateAndReadNote(t *testing.T) {
	v, _ := setupTestVault(t)

	note, err := v.CreateNote("test-note", "# Hello", map[string]any{"type": "task"})
	require.NoError(t, err)
	assert.Equal(t, "test-note", note.Path)
	assert.Equal(t, "# Hello", note.Content)
	assert.Equal(t, "task", note.Metadata["type"])

	read, err := v.ReadNote("test-note")
	require.NoError(t, err)
	assert.Equal(t, "test-note", read.Path)
	assert.Equal(t, "# Hello", read.Content)
	assert.Equal(t, "task", read.Metadata["type"])
}

func TestReadNoteNotFound(t *testing.T) {
	v, _ := setupTestVault(t)
	_, err := v.ReadNote("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpdateNote(t *testing.T) {
	v, _ := setupTestVault(t)

	_, err := v.CreateNote("test-note", "original", map[string]any{"type": "task"})
	require.NoError(t, err)

	updated, err := v.UpdateNote("test-note", "updated content", map[string]any{"type": "decision"})
	require.NoError(t, err)
	assert.Equal(t, "updated content", updated.Content)
	assert.Equal(t, "decision", updated.Metadata["type"])

	read, err := v.ReadNote("test-note")
	require.NoError(t, err)
	assert.Equal(t, "updated content", read.Content)
	assert.Equal(t, "decision", read.Metadata["type"])
}

func TestUpdateNoteNotFound(t *testing.T) {
	v, _ := setupTestVault(t)
	_, err := v.UpdateNote("nonexistent", "content", map[string]any{"type": "task"})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteNote(t *testing.T) {
	v, _ := setupTestVault(t)

	_, err := v.CreateNote("test-note", "# Hello", map[string]any{"type": "task"})
	require.NoError(t, err)

	err = v.DeleteNote("test-note")
	require.NoError(t, err)

	_, err = v.ReadNote("test-note")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteNoteNotFound(t *testing.T) {
	v, _ := setupTestVault(t)
	err := v.DeleteNote("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestListNotes(t *testing.T) {
	v, _ := setupTestVault(t)

	_, err := v.CreateNote("note1", "Content 1", map[string]any{"type": "task"})
	require.NoError(t, err)
	_, err = v.CreateNote("note2", "Content 2", map[string]any{"type": "bug"})
	require.NoError(t, err)

	notes, err := v.ListNotes("")
	require.NoError(t, err)
	assert.Len(t, notes, 2)
}

func TestListNotesWithSubdirs(t *testing.T) {
	v, _ := setupTestVault(t)

	_, err := v.CreateNote("root-note", "Root", map[string]any{"type": "task"})
	require.NoError(t, err)
	_, err = v.CreateNote("subdir/nested", "Nested", map[string]any{"type": "pattern"})
	require.NoError(t, err)

	rootNotes, err := v.ListNotes("")
	require.NoError(t, err)
	assert.Len(t, rootNotes, 1)

	subNotes, err := v.ListNotes("subdir")
	require.NoError(t, err)
	assert.Len(t, subNotes, 1)
	assert.Equal(t, "subdir/nested", subNotes[0].Path)
}

func TestListNotesDirNotFound(t *testing.T) {
	v, _ := setupTestVault(t)
	_, err := v.ListNotes("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestEnsureMakesEmptyVaultHealthyAndUpsertable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", ".agent-vault")
	v := New(root)
	require.NoError(t, v.Ensure())
	notes, err := v.ListAllNotes()
	require.NoError(t, err)
	assert.Empty(t, notes)

	_, err = v.UpsertNote("projects/demo/decision", "first", map[string]any{"type": "decision"})
	require.NoError(t, err)
	_, err = v.UpsertNote("projects/demo/decision", "second", map[string]any{"type": "decision"})
	require.NoError(t, err)
	note, err := v.ReadNote("projects/demo/decision")
	require.NoError(t, err)
	assert.Equal(t, "second", note.Content)
}

func TestPathTraversal(t *testing.T) {
	v, _ := setupTestVault(t)

	tests := []struct {
		name string
		op   func() error
	}{
		{"ReadNote", func() error { _, err := v.ReadNote("../etc/passwd"); return err }},
		{"CreateNote", func() error { _, err := v.CreateNote("../escape", "c", map[string]any{"type": "task"}); return err }},
		{"UpdateNote", func() error { _, err := v.UpdateNote("../escape", "c", map[string]any{"type": "task"}); return err }},
		{"DeleteNote", func() error { return v.DeleteNote("../escape") }},
		{"ListNotes", func() error { _, err := v.ListNotes("../"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.op()
			assert.ErrorIs(t, err, ErrPathTraversal)
		})
	}
}

func TestPathTraversal_AbsolutePath(t *testing.T) {
	v, _ := setupTestVault(t)
	_, err := v.ReadNote("/etc/passwd")
	assert.ErrorIs(t, err, ErrPathTraversal)
}

func TestConcurrentWrites(t *testing.T) {
	v, _ := setupTestVault(t)

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("note-%d", i)
			_, err := v.CreateNote(name, "content", map[string]any{"type": "task"})
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	notes, err := v.ListNotes("")
	require.NoError(t, err)
	assert.Len(t, notes, 10)
}

func TestCreateNote_AlreadyExists(t *testing.T) {
	v, _ := setupTestVault(t)

	_, err := v.CreateNote("existing", "content", map[string]any{"type": "task"})
	require.NoError(t, err)

	_, err = v.CreateNote("existing", "different", map[string]any{"type": "decision"})
	assert.Error(t, err)
}

func TestReadNote_NoFrontmatter(t *testing.T) {
	v, _ := setupTestVault(t)

	fullPath := filepath.Join(v.root, "plain.md")
	err := os.WriteFile(fullPath, []byte("Just body content"), 0644)
	require.NoError(t, err)

	note, err := v.ReadNote("plain")
	require.NoError(t, err)
	assert.Equal(t, "plain", note.Path)
	assert.Equal(t, "Just body content", note.Content)
	assert.Nil(t, note.Metadata)
}

func TestNoteWithSubdirPath(t *testing.T) {
	v, _ := setupTestVault(t)

	note, err := v.CreateNote("a/b/c/deep-note", "deep", map[string]any{"type": "session"})
	require.NoError(t, err)
	assert.Equal(t, "a/b/c/deep-note", note.Path)

	read, err := v.ReadNote("a/b/c/deep-note")
	require.NoError(t, err)
	assert.Equal(t, "deep", read.Content)
}
