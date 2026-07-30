package doctor

import (
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComprehensiveChecksCoverEverySubsystem(t *testing.T) {
	want := []string{"config", "database", "permissions", "repository", "git", "provider", "vault", "index", "ports", "daemon", "commands"}
	probes := map[string]Probe{}
	for _, name := range want {
		probes[name] = func() error { return nil }
	}
	checks := ComprehensiveChecks(probes)
	require.Len(t, checks, len(want))
	for index, name := range want {
		assert.Equal(t, name, checks[index].Name)
		assert.Equal(t, StatusPass, checks[index].Status)
	}
}

func TestDiagnosticBundleRedactsSensitiveMaterial(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "diagnostics.zip")
	err := CreateBundle(bundle, BundleInput{
		Logs:     "request Authorization: Bearer top-secret",
		Config:   "api_key: sk-secret\nmodel: local",
		Versions: map[string]string{"oberth": "0.1.0"},
		Health:   []Check{{Name: "database", Status: StatusPass}},
		Errors:   []string{"last error for token=private"},
	})
	require.NoError(t, err)

	zr, err := zip.OpenReader(bundle)
	require.NoError(t, err)
	defer zr.Close()
	var all strings.Builder
	for _, file := range zr.File {
		r, err := file.Open()
		require.NoError(t, err)
		_, _ = io.Copy(&all, r)
		_ = r.Close()
	}
	assert.NotContains(t, all.String(), "top-secret")
	assert.NotContains(t, all.String(), "sk-secret")
	assert.NotContains(t, all.String(), "private")
	assert.Contains(t, all.String(), "[REDACTED]")
}

func TestRecoveryDetectsStaleAndCanResumeOrCancel(t *testing.T) {
	now := time.Now()
	sessions := []RecoverableSession{
		{ID: "fresh", Status: "active", UpdatedAt: now.Add(-time.Minute), Artifacts: []string{"fresh.patch"}},
		{ID: "stale", Status: "active", UpdatedAt: now.Add(-time.Hour), Artifacts: []string{"stale.patch"}},
	}
	var resumed, cancelled, cleaned []string
	manager := RecoveryManager{
		StaleAfter: 10 * time.Minute,
		Now:        func() time.Time { return now },
		Resume:     func(id string) error { resumed = append(resumed, id); return nil },
		Cancel:     func(id string) error { cancelled = append(cancelled, id); return nil },
		Cleanup:    func(id string) error { cleaned = append(cleaned, id); return nil },
	}
	stale := manager.Detect(sessions)
	require.Len(t, stale, 1)
	assert.Equal(t, "stale", stale[0].ID)
	require.NoError(t, manager.Recover(stale[0], RecoveryResume))
	require.NoError(t, manager.Recover(stale[0], RecoveryCancel))
	assert.Equal(t, []string{"stale"}, resumed)
	assert.Equal(t, []string{"stale"}, cancelled)
	assert.Equal(t, []string{"stale", "stale"}, cleaned)
	assert.Equal(t, []string{"stale.patch"}, stale[0].Artifacts)
}
