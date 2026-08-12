package migration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistedFormatInventoryHasExplicitOwnersAndPolicies(t *testing.T) {
	require.ElementsMatch(t, []string{"configuration", "database", "task_state", "run_event", "result_bundle"}, formatNames())
	for _, format := range Formats {
		assert.NotEmpty(t, format.Owner)
		assert.NotEmpty(t, format.CurrentVersion)
		assert.NotEmpty(t, format.Storage)
		assert.NotEmpty(t, format.Rollback)
	}
}

func TestPriorResultBundleFixtureMigratesAndPreservesSource(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "result-bundle-v0.json"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(path, source, 0o600))

	backup, err := DefaultRegistry().MigrateFile(context.Background(), "result_bundle", path)
	require.NoError(t, err)
	assert.FileExists(t, backup)
	preserved, err := os.ReadFile(backup)
	require.NoError(t, err)
	assert.Equal(t, source, preserved)

	var migrated map[string]any
	require.NoError(t, decodeFile(path, &migrated))
	assert.Equal(t, "1", migrated["schema_version"])
	assert.NotNil(t, migrated["evidence"])
}

func TestFailedMigrationLeavesSourceRecoverable(t *testing.T) {
	source := []byte(`{"schema_version":"99","run_id":"future"}`)
	path := filepath.Join(t.TempDir(), "future.json")
	require.NoError(t, os.WriteFile(path, source, 0o600))

	backup, err := DefaultRegistry().MigrateFile(context.Background(), "result_bundle", path)
	assert.ErrorIs(t, err, ErrUnsupportedVersion)
	assert.Empty(t, backup)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, source, after)
}

func TestMigrationWillNotOverwriteConflictingRecoveryBackup(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("testdata", "result-bundle-v0.json"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(path, source, 0o600))
	require.NoError(t, os.WriteFile(path+".pre-migration-v0", []byte("other source"), 0o600))

	_, err = DefaultRegistry().MigrateFile(context.Background(), "result_bundle", path)
	assert.ErrorContains(t, err, "different content")
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, source, after)
}

func formatNames() []string {
	names := make([]string, 0, len(Formats))
	for _, format := range Formats {
		names = append(names, format.Name)
	}
	return names
}

func decodeFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !json.Valid(data) {
		return errors.New("invalid JSON")
	}
	return json.Unmarshal(data, target)
}
