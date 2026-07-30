package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverSupportsGoNodeMavenGradleAndPython(t *testing.T) {
	cases := []struct {
		name, manifest string
		want           []Phase
	}{
		{"go", "go.mod", []Phase{PhaseBuild, PhaseLint, PhaseTest}},
		{"node", "package.json", []Phase{PhaseBuild, PhaseLint, PhaseTest, PhaseTypecheck}},
		{"maven", "pom.xml", []Phase{PhaseBuild, PhaseTest}},
		{"gradle", "build.gradle", []Phase{PhaseBuild, PhaseTest}},
		{"python", "pyproject.toml", []Phase{PhaseLint, PhaseTest, PhaseTypecheck}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, tc.manifest), []byte("{}"), 0600))
			plan := Discover(root, Config{})
			for _, phase := range tc.want {
				assert.NotEmpty(t, plan.Command(phase), "missing %s", phase)
			}
		})
	}
}

func TestManualCommandsOverrideDiscovery(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), nil, 0600))
	plan := Discover(root, Config{Commands: map[Phase]string{PhaseTest: "go test ./internal/..."}})
	assert.Equal(t, "go test ./internal/...", plan.Command(PhaseTest))
}

type fakeExecutor struct {
	results map[string]Execution
	calls   []string
}

func (f *fakeExecutor) Execute(_ context.Context, command, _ string) Execution {
	f.calls = append(f.calls, command)
	result, ok := f.results[command]
	if !ok {
		return Execution{ExitCode: 0, Output: "ok", Duration: time.Millisecond}
	}
	return result
}

func TestRunnerPersistsScorecardStopsAndRerunsSelectively(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, ".oberth", "verification.json")
	plan := Plan{Root: root, ProfileVersion: "single-task/v1", DiffID: "sha256:abc", Steps: []Step{
		{Phase: PhaseBuild, Command: "build", Required: true},
		{Phase: PhaseLint, Command: "lint", Required: true},
		{Phase: PhaseTest, Command: "test", Required: true},
		{Phase: PhaseSecurity, Command: "security", Required: true},
		{Phase: PhaseDiff, Command: "diff", Required: true},
	}}
	executor := &fakeExecutor{results: map[string]Execution{"lint": {ExitCode: 1, Output: "bad lint", Duration: time.Second}}}
	runner := NewRunner(executor, store)

	scorecard, err := runner.Run(context.Background(), plan, nil)
	require.NoError(t, err)
	assert.Equal(t, "NOT_READY", scorecard.Status)
	assert.Equal(t, []string{"build", "lint"}, executor.calls)
	assert.Equal(t, "sha256:abc", scorecard.DiffID)
	assert.Equal(t, "single-task/v1", scorecard.ProfileVersion)
	loaded, err := LoadScorecard(store)
	require.NoError(t, err)
	assert.Equal(t, scorecard.Status, loaded.Status)

	executor.results["lint"] = Execution{ExitCode: 0, Output: "fixed"}
	executor.calls = nil
	_, err = runner.Run(context.Background(), plan, []Phase{PhaseLint})
	require.NoError(t, err)
	assert.Equal(t, []string{"lint"}, executor.calls)
}

type fakeReviewer struct{}

func (fakeReviewer) Review(_ context.Context, _ string) (Review, error) {
	return Review{Findings: []Finding{{Category: "regression", Severity: "high", File: "api.go", Line: 42, Evidence: "nil dereference", Suggestion: "guard nil"}}}, nil
}

func TestPatchReviewerRequiresStructuredEvidence(t *testing.T) {
	review, err := ReviewPatch(context.Background(), fakeReviewer{}, "diff --git")
	require.NoError(t, err)
	require.Len(t, review.Findings, 1)
	assert.Equal(t, "api.go", review.Findings[0].File)
	assert.Equal(t, 42, review.Findings[0].Line)
	assert.NoError(t, review.Validate())
}
