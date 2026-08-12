package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	ErrUnsupportedVersion = errors.New("persisted data version is not supported")
	ErrInvalidDocument    = errors.New("persisted data document is invalid")
)

type Step func(json.RawMessage) (json.RawMessage, error)

type Registry struct {
	steps map[string]map[string]registeredStep
}

type registeredStep struct {
	to      string
	migrate Step
}

func NewRegistry() *Registry { return &Registry{steps: map[string]map[string]registeredStep{}} }

func DefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.Register("result_bundle", "0", "1", migrateResultBundleV0)
	return registry
}

func (r *Registry) Register(format, from, to string, step Step) {
	if r.steps[format] == nil {
		r.steps[format] = map[string]registeredStep{}
	}
	r.steps[format][from] = registeredStep{to: to, migrate: step}
}

func (r *Registry) Migrate(format string, source []byte) ([]byte, string, string, error) {
	descriptor, ok := Descriptor(format)
	if !ok {
		return nil, "", "", fmt.Errorf("%w: unknown format %q", ErrUnsupportedVersion, format)
	}
	var header struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(source, &header); err != nil {
		return nil, "", "", fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if header.SchemaVersion == "" {
		header.SchemaVersion = "0"
	}
	from := header.SchemaVersion
	current := from
	data := bytes.Clone(source)
	for current != descriptor.CurrentVersion {
		step, exists := r.steps[format][current]
		if !exists {
			return nil, from, current, fmt.Errorf("%w: %s %s", ErrUnsupportedVersion, format, current)
		}
		migrated, err := step.migrate(data)
		if err != nil {
			return nil, from, current, fmt.Errorf("migrate %s %s to %s: %w", format, current, step.to, err)
		}
		if !json.Valid(migrated) {
			return nil, from, current, fmt.Errorf("%w: migration produced invalid JSON", ErrInvalidDocument)
		}
		data, current = migrated, step.to
	}
	return data, from, current, nil
}

// MigrateFile preserves the original bytes before atomically replacing a document.
func (r *Registry) MigrateFile(ctx context.Context, format, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	migrated, from, to, err := r.Migrate(format, source)
	if err != nil {
		return "", err
	}
	if bytes.Equal(source, migrated) {
		return "", nil
	}
	backup := path + ".pre-migration-v" + from
	if existing, readErr := os.ReadFile(backup); readErr == nil {
		if !bytes.Equal(existing, source) {
			return "", fmt.Errorf("recovery backup already exists with different content: %s", backup)
		}
	} else if errors.Is(readErr, os.ErrNotExist) {
		if err := writeExclusiveDurable(backup, source, 0o600); err != nil {
			return "", fmt.Errorf("preserve migration source: %w", err)
		}
	} else {
		return "", readErr
	}
	if err := writeAtomicDurable(path, migrated, 0o600); err != nil {
		return backup, fmt.Errorf("commit migrated %s %s to %s: %w", format, from, to, err)
	}
	return backup, nil
}

func migrateResultBundleV0(source json.RawMessage) (json.RawMessage, error) {
	var value map[string]any
	if err := json.Unmarshal(source, &value); err != nil {
		return nil, err
	}
	value["schema_version"] = "1"
	if _, ok := value["evidence"]; !ok {
		value["evidence"] = []any{}
	}
	return json.MarshalIndent(value, "", "  ")
}

func writeExclusiveDurable(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func writeAtomicDurable(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	tmp, err := os.CreateTemp(directory, ".oberth-migration-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
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
	return os.Rename(tmpName, path)
}
