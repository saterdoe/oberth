package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigCheck_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".oberth.yaml")
	require.NoError(t, os.WriteFile(path, []byte("key: value\n"), 0644))

	c := ConfigCheck(path)
	assert.Equal(t, StatusPass, c.Status)
}

func TestConfigCheck_Missing(t *testing.T) {
	c := ConfigCheck("/nonexistent/config.yaml")
	assert.Equal(t, StatusFail, c.Status)
}

func TestRuntimeConfigCheck_MissingUsesDefaults(t *testing.T) {
	c := RuntimeConfigCheck(filepath.Join(t.TempDir(), ".oberth.yaml"))
	assert.Equal(t, StatusPass, c.Status)
	assert.Contains(t, c.Message, "defaults")
}

func TestRuntimeConfigCheck_RejectsDirectory(t *testing.T) {
	c := RuntimeConfigCheck(t.TempDir())
	assert.Equal(t, StatusFail, c.Status)
}

func TestConfigCheck_IsDir(t *testing.T) {
	tmpDir := t.TempDir()
	c := ConfigCheck(tmpDir)
	assert.Equal(t, StatusFail, c.Status)
}

func TestVaultStructureCheck_AllDirs(t *testing.T) {
	tmpDir := t.TempDir()
	for _, dir := range []string{"architecture", "decisions", "patterns", "bugs", "sessions", "tasks"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, dir), 0755))
	}

	c := VaultStructureCheck(tmpDir)
	assert.Equal(t, StatusPass, c.Status)
}

func TestVaultStructureCheck_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "architecture"), 0755))

	c := VaultStructureCheck(tmpDir)
	assert.Equal(t, StatusWarn, c.Status)
}

func TestVaultStructureCheck_NotExists(t *testing.T) {
	c := VaultStructureCheck("/nonexistent/vault")
	assert.Equal(t, StatusFail, c.Status)
}

func TestAllChecks(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".oberth.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("key: value\n"), 0644))
	for _, dir := range []string{"architecture", "decisions", "patterns", "bugs", "sessions", "tasks"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault", dir), 0755))
	}

	checks := AllChecks(configPath, filepath.Join(tmpDir, ".agent-vault"))
	for _, c := range checks {
		assert.Equal(t, StatusPass, c.Status, "check %s should pass", c.Name)
	}
}
