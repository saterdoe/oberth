package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCmd_CreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	rootCmd.SetArgs([]string{"init"})
	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault"))
	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault", "architecture"))
	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault", "decisions"))
	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault", "patterns"))
	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault", "bugs"))
	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault", "sessions"))
	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault", "tasks"))
	assert.FileExists(t, filepath.Join(tmpDir, ".agent-vault", "memory-index.md"))
	assert.FileExists(t, filepath.Join(tmpDir, ".oberth.yaml"))
	config, err := os.ReadFile(filepath.Join(tmpDir, ".oberth.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(config), "host: 127.0.0.1")
	assert.Contains(t, string(config), "driver: embedded")
	assert.Contains(t, string(config), "mode: token")
	assert.NotContains(t, string(config), "postgres://")
	assert.NotContains(t, string(config), "host: 0.0.0.0")
}

func TestInitCmd_FailsWhenExists(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault"), 0755))

	rootCmd.SetArgs([]string{"init"})
	err := rootCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestInitCmd_ForceOverwrites(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault"), 0755))

	rootCmd.SetArgs([]string{"init", "--force"})
	err := rootCmd.Execute()
	require.NoError(t, err)

	assert.DirExists(t, filepath.Join(tmpDir, ".agent-vault", "architecture"))
}
