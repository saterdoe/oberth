package toolrunner

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saterdoe/oberth/internal/permission"
)

type Limits struct {
	MaxFiles   int
	MaxBytes   int
	MaxMatches int
}

type Match struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type Reader struct {
	root   string
	guard  *permission.WorkspaceGuard
	limits Limits
}

func NewReader(root string, guard *permission.WorkspaceGuard, limits Limits) *Reader {
	return &Reader{root: root, guard: guard, limits: limits}
}

func (r *Reader) target(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(r.root, path)
}

func (r *Reader) List(path string) ([]string, error) {
	target := r.target(path)
	if err := r.guard.Authorize(target); err != nil {
		return nil, err
	}
	var files []string
	err := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(r.root, path)
		files = append(files, filepath.ToSlash(rel))
		if r.limits.MaxFiles > 0 && len(files) >= r.limits.MaxFiles {
			return fs.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func (r *Reader) Read(path string) (string, bool, error) {
	target := r.target(path)
	if err := r.guard.Authorize(target); err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", false, err
	}
	if r.limits.MaxBytes > 0 && len(data) > r.limits.MaxBytes {
		return string(data[:r.limits.MaxBytes]), true, nil
	}
	return string(data), false, nil
}

func (r *Reader) Search(query string) ([]Match, error) {
	files, err := r.List(".")
	if err != nil {
		return nil, err
	}
	var matches []Match
	for _, path := range files {
		data, _, err := r.Read(path)
		if err != nil {
			continue
		}
		for index, line := range strings.Split(data, "\n") {
			if strings.Contains(line, query) {
				matches = append(matches, Match{path, index + 1, line})
				if r.limits.MaxMatches > 0 && len(matches) >= r.limits.MaxMatches {
					return matches, nil
				}
			}
		}
	}
	return matches, nil
}

func (r *Reader) Inspect(path string) error {
	target := r.target(path)
	if err := r.guard.Authorize(target); err != nil {
		return err
	}
	_, err := os.Stat(target)
	return err
}

type Writer struct {
	root    string
	guard   *permission.WorkspaceGuard
	backups map[string]map[string][]byte
}

func NewWriter(root string, guard *permission.WorkspaceGuard) *Writer {
	return &Writer{root: root, guard: guard, backups: map[string]map[string][]byte{}}
}

func (w *Writer) target(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(w.root, path)
}

func (w *Writer) Create(path string, content []byte) error {
	target := w.target(path)
	if err := w.guard.Authorize(target); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("file already exists")
	}
	return atomicWrite(target, content, 0600)
}

func (w *Writer) ReplaceRange(session, path string, start, end int, replacement []byte) error {
	target := w.target(path)
	if err := w.guard.Authorize(target); err != nil {
		return err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if start < 0 || end < start || end > len(data) {
		return fmt.Errorf("invalid replacement range")
	}
	if w.backups[session] == nil {
		w.backups[session] = map[string][]byte{}
	}
	if _, exists := w.backups[session][target]; !exists {
		w.backups[session][target] = bytes.Clone(data)
	}
	updated := append(bytes.Clone(data[:start]), replacement...)
	updated = append(updated, data[end:]...)
	return atomicWrite(target, updated, 0600)
}

func (w *Writer) Revert(session string) error {
	for path, content := range w.backups[session] {
		if err := atomicWrite(path, content, 0600); err != nil {
			return err
		}
	}
	delete(w.backups, session)
	return nil
}

func atomicWrite(target string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".oberth-write-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(name, target)
}
