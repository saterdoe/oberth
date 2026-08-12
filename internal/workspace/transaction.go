package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type OperationKind string

const (
	OperationCreate  OperationKind = "create"
	OperationReplace OperationKind = "replace"
	OperationRename  OperationKind = "rename"
	OperationDelete  OperationKind = "delete"
)

var ErrInjectedCrash = errors.New("injected workspace transaction crash")

type Operation struct {
	Kind    OperationKind `json:"kind"`
	Path    string        `json:"path"`
	To      string        `json:"to,omitempty"`
	Content []byte        `json:"content,omitempty"`
}

type beforeImage struct {
	Path   string      `json:"path"`
	Exists bool        `json:"exists"`
	Mode   os.FileMode `json:"mode,omitempty"`
	Data   []byte      `json:"data,omitempty"`
}

type transactionManifest struct {
	ID         string        `json:"id"`
	State      string        `json:"state"`
	Applied    int           `json:"applied"`
	Operations []Operation   `json:"operations"`
	Before     []beforeImage `json:"before"`
}

func (s *Service) transactionRoot() string {
	return filepath.Join(filepath.Dir(s.root), "."+filepath.Base(s.root)+".oberth-transactions")
}

func (s *Service) ApplyOperations(ctx context.Context, id string, operations []Operation) error {
	if strings.TrimSpace(id) == "" || len(operations) == 0 {
		return errors.New("transaction id and operations are required")
	}
	if filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return errors.New("transaction id must be a safe filename")
	}
	dir := filepath.Join(s.transactionRoot(), id)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("transaction %q already exists", id)
	}
	manifest := transactionManifest{ID: id, State: "prepared", Operations: operations}
	paths := map[string]struct{}{}
	for _, operation := range operations {
		paths[operation.Path] = struct{}{}
		if operation.To != "" {
			paths[operation.To] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		target, err := s.resolve(path)
		if err != nil {
			return err
		}
		image := beforeImage{Path: path}
		info, err := os.Stat(target)
		if err == nil {
			if info.IsDir() {
				return fmt.Errorf("transaction target %q is a directory", path)
			}
			image.Exists, image.Mode = true, info.Mode().Perm()
			image.Data, err = os.ReadFile(target)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		manifest.Before = append(manifest.Before, image)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := writeManifest(dir, manifest); err != nil {
		return err
	}
	manifest.State = "applying"
	if err := writeManifest(dir, manifest); err != nil {
		return err
	}
	for index, operation := range operations {
		if err := s.applyOperation(ctx, operation); err != nil {
			_ = s.restoreBeforeImages(context.WithoutCancel(ctx), manifest.Before)
			_ = os.RemoveAll(dir)
			return err
		}
		manifest.Applied = index + 1
		if err := writeManifest(dir, manifest); err != nil {
			return err
		}
		if s.transactionHook != nil {
			if err := s.transactionHook(fmt.Sprintf("after_%s", operation.Kind)); err != nil {
				return fmt.Errorf("%w: %v", ErrInjectedCrash, err)
			}
		}
	}
	manifest.State = "committed"
	if err := writeManifest(dir, manifest); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (s *Service) applyOperation(ctx context.Context, operation Operation) error {
	switch operation.Kind {
	case OperationCreate:
		return s.Create(ctx, operation.Path, operation.Content)
	case OperationReplace:
		return s.WriteExisting(ctx, operation.Path, operation.Content)
	case OperationDelete:
		target, err := s.resolve(operation.Path)
		if err != nil {
			return err
		}
		return os.Remove(target)
	case OperationRename:
		from, err := s.resolve(operation.Path)
		if err != nil {
			return err
		}
		to, err := s.resolve(operation.To)
		if err != nil {
			return err
		}
		if _, err := os.Stat(to); err == nil {
			return fmt.Errorf("rename destination %q exists", operation.To)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
			return err
		}
		return os.Rename(from, to)
	default:
		return fmt.Errorf("unsupported workspace operation %q", operation.Kind)
	}
}

func (s *Service) RecoverTransactions(ctx context.Context) error {
	entries, err := os.ReadDir(s.transactionRoot())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(s.transactionRoot(), entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			return err
		}
		var manifest transactionManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return err
		}
		if manifest.State != "committed" {
			if err := s.restoreBeforeImages(ctx, manifest.Before); err != nil {
				return err
			}
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) restoreBeforeImages(ctx context.Context, images []beforeImage) error {
	for _, image := range images {
		if err := ctx.Err(); err != nil {
			return err
		}
		target, err := s.resolve(image.Path)
		if err != nil {
			return err
		}
		if !image.Exists {
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			if err := s.Create(ctx, image.Path, image.Data); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := s.WriteExisting(ctx, image.Path, image.Data); err != nil {
			return err
		}
		if err := os.Chmod(target, image.Mode); err != nil {
			return err
		}
	}
	return nil
}

func writeManifest(dir string, manifest transactionManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".manifest-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
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
	target := filepath.Join(dir, "manifest.json")
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return os.Rename(name, target)
	} else if err != nil {
		return err
	}
	return replaceFile(name, target)
}
