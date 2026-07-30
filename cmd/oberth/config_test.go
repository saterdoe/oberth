package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShowConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".oberth.yaml")
	content := "server:\n  host: 0.0.0.0\n  port: 9090\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))

	err := showConfig(cfgPath)
	assert.NoError(t, err)
}

func TestShowConfig_MissingFile(t *testing.T) {
	err := showConfig("/nonexistent/.oberth.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
}

func TestValidateConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".oberth.yaml")
	content := "server:\n  host: 0.0.0.0\n  port: 9090\nvault:\n  path: ./.agent-vault\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))

	err := validateConfig(cfgPath)
	assert.NoError(t, err)
}

func TestValidateConfig_MissingFile(t *testing.T) {
	err := validateConfig("/nonexistent/.oberth.yaml")
	assert.Error(t, err)
}

func TestValidateConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".oberth.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(": invalid yaml"), 0644))

	err := validateConfig(cfgPath)
	assert.Error(t, err)
}

func TestConfigShowCmd(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.WriteFile(".oberth.yaml", []byte("key: value\n"), 0644))

	rootCmd.SetArgs([]string{"config", "show"})
	err := rootCmd.Execute()
	assert.NoError(t, err)
}

func TestConfigValidateCmd(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.WriteFile(".oberth.yaml", []byte("server:\n  host: 0.0.0.0\n  port: 9090\nvault:\n  path: ./.agent-vault\n"), 0644))

	rootCmd.SetArgs([]string{"config", "validate"})
	err := rootCmd.Execute()
	assert.NoError(t, err)
}

func TestConfigShowCmd_MissingFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	rootCmd.SetArgs([]string{"config", "show"})
	err := rootCmd.Execute()
	assert.Error(t, err)
}
