package context_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/stretchr/testify/require"
)

func TestCompileWithOptionsEnforcesBudgetAndOutputReserve(t *testing.T) {
	v, _ := setupTestVault(t)
	p := pkgctx.NewPipeline(v, nil)
	result, err := p.CompileWithOptions(context.Background(), "implement handler", "implementation", pkgctx.CompileOptions{MaxTokens: 100, ReserveOutputTokens: 40, RepoSources: []pkgctx.ContextSource{{ID: "repo:a", Kind: "code", Content: strings.Repeat("a", 400)}, {ID: "repo:b", Kind: "code", Content: strings.Repeat("b", 400)}}})
	require.NoError(t, err)
	require.LessOrEqual(t, result.Tokens, 60)
	require.Equal(t, 40, result.Metrics.ReservedOutputTokens)
	require.Greater(t, result.Metrics.Dropped, 0)
}

func TestCompileRepositoryIncludesRepoMapAndMatchingCode(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "internal", "auth"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "internal", "auth", "login.go"), []byte("package auth\nfunc ValidateLogin() {}\n"), 0o644))
	v, _ := setupTestVault(t)
	p := pkgctx.NewPipeline(v, nil)
	result, err := p.CompileRepository(context.Background(), root, "ValidateLogin", "bug_fix", pkgctx.CompileOptions{MaxTokens: 300})
	require.NoError(t, err)
	require.Contains(t, result.Context, "Primary language: Go")
	require.Contains(t, result.Context, "ValidateLogin")
	require.Contains(t, result.Sources, "repository-map")
}

func TestCompileWithOptionsSelectsSourcesByTaskAndReportsMetrics(t *testing.T) {
	v, _ := setupTestVault(t)
	p := pkgctx.NewPipeline(v, nil)
	result, err := p.CompileWithOptions(context.Background(), "fix login bug", "bug_fix", pkgctx.CompileOptions{MaxTokens: 300, RepoSources: []pkgctx.ContextSource{{ID: "bug.go", Kind: "code", TaskTypes: []string{"bug_fix"}, Content: "login bug handler"}, {ID: "docs.md", Kind: "docs", TaskTypes: []string{"docs"}, Content: "documentation"}}})
	require.NoError(t, err)
	require.Contains(t, result.Sources, "bug.go")
	require.NotContains(t, result.Sources, "docs.md")
	require.Equal(t, 2, result.Metrics.Candidates)
	require.Equal(t, 1, result.Metrics.Selected)
}

func TestCompileWithOptionsExpandsIterativelyWithinRemainingBudget(t *testing.T) {
	v, _ := setupTestVault(t)
	p := pkgctx.NewPipeline(v, nil)
	calls := 0
	result, err := p.CompileWithOptions(context.Background(), "review service", "review", pkgctx.CompileOptions{MaxTokens: 200, Expand: func(_ context.Context, remaining int) ([]pkgctx.ContextSource, error) {
		calls++
		require.Greater(t, remaining, 0)
		return []pkgctx.ContextSource{{ID: "dependency.go", Kind: "dependency", Content: "func dependency() {}"}}, nil
	}})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Contains(t, result.Sources, "dependency.go")
	require.Equal(t, 1, result.Metrics.ExpansionRounds)
}

func TestCompileMetricsCompareEquivalentSerializedContext(t *testing.T) {
	v, _ := setupTestVault(t)
	p := pkgctx.NewPipeline(v, nil)
	result, err := p.CompileWithOptions(context.Background(), "inspect repository", "implementation", pkgctx.CompileOptions{
		MaxTokens: 300,
		RepoSources: []pkgctx.ContextSource{
			{ID: "repository-map", Kind: "repo_map", Content: "small map"},
			{ID: "repository-metadata", Kind: "metadata", Content: "small metadata"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, result.Metrics.CandidateTokens, result.Metrics.SelectedTokens)
	require.Equal(t, 0, result.Metrics.SavedTokens)
	require.Equal(t, float64(0), result.Metrics.SavingsPercent)
}

func TestCompileWithOptionsCompactsOversizedHighPrioritySource(t *testing.T) {
	v, _ := setupTestVault(t)
	p := pkgctx.NewPipeline(v, nil)
	result, err := p.CompileWithOptions(context.Background(), "review", "review", pkgctx.CompileOptions{MaxTokens: 40, RepoSources: []pkgctx.ContextSource{{ID: "large.go", Kind: "code", Content: "START\n" + strings.Repeat("implementation detail ", 60) + "\nEND"}}})
	require.NoError(t, err)
	require.Contains(t, result.Sources, "large.go")
	require.Contains(t, result.Context, "START")
	require.Contains(t, result.Context, "END")
	require.Equal(t, 1, result.Metrics.Compacted)
	require.LessOrEqual(t, result.Tokens, 40)
}
