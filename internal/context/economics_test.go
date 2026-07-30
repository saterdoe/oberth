package context_test

import (
	"context"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompilerRanksDeduplicatesDiversifiesAndExplainsSelection(t *testing.T) {
	v, _ := setupTestVault(t)
	pipeline := pkgctx.NewPipeline(v, nil)
	duplicate := "critical login validation and authorization logic"
	result, err := pipeline.CompileWithOptions(context.Background(), "fix login authorization", "bug_fix", pkgctx.CompileOptions{
		MaxTokens: 80, MaxSourcesPerKind: 1,
		RepoSources: []pkgctx.ContextSource{
			{ID: "noise.md", Kind: "docs", Content: strings.Repeat("unrelated prose ", 20), Priority: 1, Relevance: .1},
			{ID: "auth.go", Kind: "code", Content: duplicate, Priority: 100, Relevance: .95, Reason: "explicit symbol match"},
			{ID: "auth-copy.go", Kind: "code", Content: duplicate, Priority: 90, Relevance: .9},
			{ID: "decision.md", Kind: "memory", Content: "authorization decision for login", Priority: 70, Relevance: .8},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Sources)
	assert.Equal(t, "auth.go", result.Sources[0])
	assert.Contains(t, result.Sources, "decision.md")
	assert.NotContains(t, result.Sources, "auth-copy.go")
	assert.GreaterOrEqual(t, result.Metrics.DuplicateDropped, 1)
	assert.Greater(t, result.Metrics.SavingsPercent, float64(0))
	assert.Greater(t, result.Metrics.BudgetUtilizationPercent, float64(0))
	assert.Equal(t, 1, result.Metrics.SelectedByKind["code"])
	require.NotEmpty(t, result.Manifest)
	assert.Equal(t, "explicit symbol match", result.Manifest[0].Reason)
}

func TestCompilationCacheHitsAndInvalidatesOnContent(t *testing.T) {
	v, _ := setupTestVault(t)
	pipeline := pkgctx.NewPipeline(v, nil)
	cache := pkgctx.NewCompilationCache(4, time.Hour)
	options := pkgctx.CompileOptions{MaxTokens: 100, Cache: cache, RepoSources: []pkgctx.ContextSource{{ID: "a", Kind: "code", Content: "alpha", Priority: 1}}}
	first, err := pipeline.CompileWithOptions(context.Background(), "alpha", "implementation", options)
	require.NoError(t, err)
	second, err := pipeline.CompileWithOptions(context.Background(), "alpha", "implementation", options)
	require.NoError(t, err)
	assert.False(t, first.Metrics.CacheHit)
	assert.True(t, second.Metrics.CacheHit)

	options.RepoSources[0].Content = "alpha changed"
	third, err := pipeline.CompileWithOptions(context.Background(), "alpha", "implementation", options)
	require.NoError(t, err)
	assert.False(t, third.Metrics.CacheHit)
}

func TestExpansionIsBoundedAndDeduplicatedAcrossRounds(t *testing.T) {
	v, _ := setupTestVault(t)
	pipeline := pkgctx.NewPipeline(v, nil)
	round := 0
	result, err := pipeline.CompileWithOptions(context.Background(), "trace dependency", "review", pkgctx.CompileOptions{
		MaxTokens: 300, MaxExpansionRounds: 3,
		Expand: func(_ context.Context, _ int) ([]pkgctx.ContextSource, error) {
			round++
			return []pkgctx.ContextSource{{ID: "dep-" + string(rune('0'+round)), Kind: "dependency", Content: "dependency round " + string(rune('0'+round)), Priority: 10}}, nil
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, round)
	assert.Equal(t, 3, result.Metrics.ExpansionRounds)
	assert.Len(t, result.Sources, 3)
}

func TestContextModesHaveDistinctEconomicProfiles(t *testing.T) {
	dev, err := pkgctx.ProfileForMode(pkgctx.ModeDev)
	require.NoError(t, err)
	review, err := pkgctx.ProfileForMode(pkgctx.ModeReview)
	require.NoError(t, err)
	research, err := pkgctx.ProfileForMode(pkgctx.ModeResearch)
	require.NoError(t, err)
	assert.NotEqual(t, dev.MaxTokens, review.MaxTokens)
	assert.Greater(t, research.MaxTokens, dev.MaxTokens)
	assert.Contains(t, review.PreferredKinds, "diff")
	assert.Contains(t, research.PreferredKinds, "docs")
}

func TestEvalHarnessMeasuresRecallNoiseAndTokenSavings(t *testing.T) {
	v, _ := setupTestVault(t)
	pipeline := pkgctx.NewPipeline(v, nil)
	report, err := pkgctx.RunEvals(context.Background(), pipeline, []pkgctx.EvalCase{{
		Name: "auth", Query: "login auth", TaskType: "bug_fix", ExpectedSources: []string{"auth.go"},
		Options: pkgctx.CompileOptions{MaxTokens: 80, RepoSources: []pkgctx.ContextSource{
			{ID: "auth.go", Kind: "code", Content: "login auth validation", Priority: 100, Relevance: 1},
			{ID: "noise.md", Kind: "docs", Content: strings.Repeat("noise ", 80), Priority: 1},
		}},
	}}, pkgctx.EvalThresholds{MinRecall: 1, MaxNoiseRatio: .5, MinSavingsPercent: 10})
	require.NoError(t, err)
	assert.True(t, report.Passed)
	assert.Equal(t, float64(1), report.AverageRecall)
	assert.GreaterOrEqual(t, report.AverageSavingsPercent, float64(10))
}
