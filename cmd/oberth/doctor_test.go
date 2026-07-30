package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoctorCmd_WithValidSetup(t *testing.T) {
	t.Setenv("OBERTH_DOCTOR_DAEMON_URL", "http://127.0.0.1:1/api/v1/health")
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault", "architecture"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault", "decisions"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault", "patterns"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault", "bugs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault", "sessions"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault", "tasks"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".oberth.yaml"), []byte("server:\n  host: 0.0.0.0\n  port: 9090\n"), 0644))

	rootCmd.SetArgs([]string{"doctor"})
	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestDoctorCmd_WithMissingConfig(t *testing.T) {
	t.Setenv("OBERTH_DOCTOR_DAEMON_URL", "http://127.0.0.1:1/api/v1/health")
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	rootCmd.SetArgs([]string{"doctor"})
	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestDoctorCmd_WithMissingVaultDirs(t *testing.T) {
	t.Setenv("OBERTH_DOCTOR_DAEMON_URL", "http://127.0.0.1:1/api/v1/health")
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, ".agent-vault"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".oberth.yaml"), []byte("server:\n  host: 0.0.0.0\n"), 0644))

	rootCmd.SetArgs([]string{"doctor"})
	err := rootCmd.Execute()
	require.Error(t, err)
}
