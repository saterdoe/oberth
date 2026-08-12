package workspace

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/saterdoe/oberth/internal/permission"
)

var ErrAbsolutePath = errors.New("workspace paths must be relative")

// Service is the only filesystem boundary exposed to application code.
// Callers provide relative paths; the service validates traversal and symlinks.
type Service struct {
	root            string
	guard           *permission.WorkspaceGuard
	transactionHook func(string) error
}

type Change struct {
	Path         string `json:"path"`
	Content      []byte `json:"content"`
	ExpectedHash string `json:"expected_hash,omitempty"`
}

type ChangeSet struct {
	ID      string            `json:"id"`
	Before  map[string][]byte `json:"-"`
	Created map[string]bool   `json:"-"`
	Changes []string          `json:"changes"`
}

func New(root string) (*Service, error) {
	guard, err := permission.NewWorkspaceGuard(root)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	service := &Service{root: filepath.Clean(abs), guard: guard}
	if err := service.RecoverTransactions(context.Background()); err != nil {
		return nil, fmt.Errorf("recover workspace transactions: %w", err)
	}
	return service, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) Relative(target string) (string, error) {
	if !filepath.IsAbs(target) {
		target = filepath.Join(s.root, target)
	}
	if err := s.guard.Authorize(target); err != nil {
		return "", err
	}
	relative, err := filepath.Rel(s.root, target)
	if err != nil {
		return "", err
	}
	return filepath.Clean(relative), nil
}

func (s *Service) resolve(relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", ErrAbsolutePath
	}
	clean := filepath.Clean(strings.TrimSpace(relativePath))
	if clean == "." || clean == "" {
		return s.root, nil
	}
	target := filepath.Join(s.root, clean)
	if err := s.guard.Authorize(target); err != nil {
		return "", err
	}
	return target, nil
}

func (s *Service) List(ctx context.Context, relativePath string) ([]fs.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target, err := s.resolve(relativePath)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(target)
}

func (s *Service) Read(ctx context.Context, relativePath string) ([]byte, fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	target, err := s.resolve(relativePath)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("%s is a directory", relativePath)
	}
	data, err := os.ReadFile(target)
	return data, info, err
}

func (s *Service) WriteExisting(ctx context.Context, relativePath string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := s.resolve(relativePath)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", relativePath)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".oberth-workspace-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
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
	return replaceFile(tmpName, target)
}

func (s *Service) Create(ctx context.Context, relativePath string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := s.resolve(relativePath)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("%s already exists", relativePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(target)
	if err := s.guard.Authorize(parent); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parent, ".oberth-workspace-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
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
	return os.Rename(tmpName, target)
}

func (s *Service) Apply(ctx context.Context, id string, changes []Change) (ChangeSet, error) {
	set := ChangeSet{ID: id, Before: map[string][]byte{}, Created: map[string]bool{}}
	for _, change := range changes {
		current, _, err := s.Read(ctx, change.Path)
		if errors.Is(err, os.ErrNotExist) && change.ExpectedHash == "" {
			set.Created[change.Path] = true
			continue
		}
		if err != nil {
			return ChangeSet{}, err
		}
		if change.ExpectedHash != "" && change.ExpectedHash != hash(current) {
			return ChangeSet{}, fmt.Errorf("content conflict: %s", change.Path)
		}
		set.Before[change.Path] = append([]byte(nil), current...)
	}
	operations := make([]Operation, 0, len(changes))
	for _, change := range changes {
		kind := OperationReplace
		if set.Created[change.Path] {
			kind = OperationCreate
		}
		operations = append(operations, Operation{Kind: kind, Path: change.Path, Content: change.Content})
	}
	if err := s.ApplyOperations(ctx, id, operations); err != nil {
		return ChangeSet{}, err
	}
	for _, change := range changes {
		set.Changes = append(set.Changes, change.Path)
	}
	return set, nil
}

func (s *Service) Rollback(ctx context.Context, set ChangeSet) error {
	var rollbackErr error
	for path := range set.Created {
		target, err := s.resolve(path)
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
			continue
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	for path, content := range set.Before {
		if err := s.WriteExisting(ctx, path, content); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func hash(data []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
