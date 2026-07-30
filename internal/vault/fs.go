package vault

import (
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// Note represents a markdown note with frontmatter metadata.
type Note struct {
	Path     string         `json:"path"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

// Vault provides filesystem access to a vault of markdown notes.
// Each note is stored as a .md file with optional YAML frontmatter.
type Vault struct {
	root string
}

// New creates a new Vault rooted at the given directory.
func New(root string) *Vault {
	return &Vault{root: root}
}

// Ensure creates the canonical vault structure. An empty vault is a valid,
// healthy state and must never surface as a read failure.
func (v *Vault) Ensure() error {
	for _, dir := range []string{"architecture", "bugs", "decisions", "patterns", "projects", "sessions", "tasks"} {
		if err := os.MkdirAll(filepath.Join(v.root, dir), 0700); err != nil {
			return err
		}
	}
	return nil
}

func (v *Vault) Root() string { return v.root }

// UpsertNote creates or atomically replaces a note.
func (v *Vault) UpsertNote(path string, content string, metadata map[string]any) (*Note, error) {
	if _, err := v.ReadNote(path); err == nil {
		return v.UpdateNote(path, content, metadata)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return v.CreateNote(path, content, metadata)
}

// safePath resolves path relative to vault root, preventing traversal.
func (v *Vault) safePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		return v.root, nil
	}
	if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) || pathpkg.IsAbs(filepath.ToSlash(path)) {
		return "", ErrPathTraversal
	}

	fullPath := filepath.Join(v.root, cleanPath)

	absRoot, err := filepath.Abs(v.root)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(absFull, absRoot) {
		return "", ErrPathTraversal
	}

	return fullPath, nil
}

// ReadNote reads a note from the vault by its path (without .md extension).
func (v *Vault) ReadNote(path string) (*Note, error) {
	fullPath, err := v.safePath(path + ".md")
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	metadata, body, err := ParseFrontmatter(string(data))
	if err != nil {
		return nil, err
	}

	return &Note{
		Path:     path,
		Content:  body,
		Metadata: metadata,
	}, nil
}

// CreateNote creates a new note in the vault.
func (v *Vault) CreateNote(path string, content string, metadata map[string]any) (*Note, error) {
	fullPath, err := v.safePath(path + ".md")
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, err
	}

	if _, err := os.Stat(fullPath); err == nil {
		return nil, fmt.Errorf("vault: note already exists: %s", path)
	}

	written, err := WriteFrontmatter(metadata, content)
	if err != nil {
		return nil, err
	}

	if err := writeAtomic(fullPath, []byte(written), 0644, false); err != nil {
		return nil, err
	}

	return &Note{
		Path:     path,
		Content:  content,
		Metadata: metadata,
	}, nil
}

// UpdateNote updates an existing note in the vault.
func (v *Vault) UpdateNote(path string, content string, metadata map[string]any) (*Note, error) {
	fullPath, err := v.safePath(path + ".md")
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, ErrNotFound
	}

	written, err := WriteFrontmatter(metadata, content)
	if err != nil {
		return nil, err
	}

	if err := writeAtomic(fullPath, []byte(written), 0644, true); err != nil {
		return nil, err
	}

	return &Note{
		Path:     path,
		Content:  content,
		Metadata: metadata,
	}, nil
}

func writeAtomic(target string, data []byte, mode os.FileMode, replace bool) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".oberth-note-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpPath) }
	defer cleanup()
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if !replace {
		return os.Rename(tmpPath, target)
	}
	backup := target + ".oberth-backup"
	_ = os.Remove(backup)
	if err = os.Rename(target, backup); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	return os.Remove(backup)
}

// DeleteNote deletes a note from the vault.
func (v *Vault) DeleteNote(path string) error {
	fullPath, err := v.safePath(path + ".md")
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

// ListNotes lists all notes in the given directory (non-recursive).
func (v *Vault) ListNotes(dir string) ([]Note, error) {
	fullDir, err := v.safePath(dir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var notes []Note
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		notePath := pathpkg.Join(filepath.ToSlash(dir), entry.Name())
		note, err := v.ReadNote(strings.TrimSuffix(notePath, ".md"))
		if err != nil {
			return nil, err
		}
		notes = append(notes, *note)
	}

	return notes, nil
}

// ListAllNotes lists every markdown note under the vault root recursively.
func (v *Vault) ListAllNotes() ([]Note, error) {
	root, err := v.safePath("")
	if err != nil {
		return nil, err
	}

	var notes []Note
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		notePath := strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		note, err := v.ReadNote(notePath)
		if err != nil {
			return err
		}
		notes = append(notes, *note)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return notes, nil
}
