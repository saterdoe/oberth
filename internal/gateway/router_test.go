package gateway

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db/repos"
)

func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `TRUNCATE routing_rules, providers CASCADE`)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func createTestProvider(t *testing.T, repo *repos.ProviderRepo) *repos.Provider {
	t.Helper()
	p := &repos.Provider{
		Name:         "router-test-provider-" + uuid.NewString()[:8],
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
		Priority:     0,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	return p
}

func TestMatch_NoRules(t *testing.T) {
	pool := setupTestPool(t)
	ruleRepo := repos.NewRoutingRuleRepo(pool)
	providerRepo := repos.NewProviderRepo(pool)
	router := NewRouter(ruleRepo, providerRepo)
	ctx := context.Background()

	result, err := router.Match(ctx, RouteRequest{
		RepoPath: "org/repo",
		TaskType: "review",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestMatch_WithMatchingRule(t *testing.T) {
	pool := setupTestPool(t)
	prepo := repos.NewProviderRepo(pool)
	rrepo := repos.NewRoutingRuleRepo(pool)
	router := NewRouter(rrepo, prepo)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)

	rule := &repos.RoutingRule{
		Name:       "match-test-" + uuid.NewString()[:8],
		Priority:   1,
		IsActive:   true,
		ProviderID: provider.ID,
		Model:      "gpt-4",
	}
	require.NoError(t, rrepo.Create(ctx, rule))

	result, err := router.Match(ctx, RouteRequest{
		RepoPath: "org/repo",
		TaskType: "review",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, rule.ID, result.Rule.ID)
	assert.Equal(t, provider.ID, result.Provider.ID)
	assert.Nil(t, result.ExecutionGraph)
}

func TestMatch_WithRepoPatternGlob(t *testing.T) {
	pool := setupTestPool(t)
	prepo := repos.NewProviderRepo(pool)
	rrepo := repos.NewRoutingRuleRepo(pool)
	router := NewRouter(rrepo, prepo)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)

	pattern := "org/*"
	rule := &repos.RoutingRule{
		Name:             "glob-test-" + uuid.NewString()[:8],
		Priority:         1,
		IsActive:         true,
		MatchRepoPattern: &pattern,
		ProviderID:       provider.ID,
		Model:            "claude-3",
	}
	require.NoError(t, rrepo.Create(ctx, rule))

	result, err := router.Match(ctx, RouteRequest{
		RepoPath: "org/my-repo",
		TaskType: "review",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, rule.ID, result.Rule.ID)

	result, err = router.Match(ctx, RouteRequest{
		RepoPath: "other/repo",
		TaskType: "review",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestMatch_WithTaskType(t *testing.T) {
	pool := setupTestPool(t)
	prepo := repos.NewProviderRepo(pool)
	rrepo := repos.NewRoutingRuleRepo(pool)
	router := NewRouter(rrepo, prepo)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)

	taskType := "code-review"
	rule := &repos.RoutingRule{
		Name:          "task-test-" + uuid.NewString()[:8],
		Priority:      1,
		IsActive:      true,
		MatchTaskType: &taskType,
		ProviderID:    provider.ID,
		Model:         "gpt-4",
	}
	require.NoError(t, rrepo.Create(ctx, rule))

	result, err := router.Match(ctx, RouteRequest{
		RepoPath: "org/repo",
		TaskType: "code-review",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, rule.ID, result.Rule.ID)

	result, err = router.Match(ctx, RouteRequest{
		RepoPath: "org/repo",
		TaskType: "generate",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestMatch_NoRulesMatch(t *testing.T) {
	pool := setupTestPool(t)
	prepo := repos.NewProviderRepo(pool)
	rrepo := repos.NewRoutingRuleRepo(pool)
	router := NewRouter(rrepo, prepo)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)

	taskType := "specific-task"
	rule := &repos.RoutingRule{
		Name:          "no-match-" + uuid.NewString()[:8],
		Priority:      1,
		IsActive:      true,
		MatchTaskType: &taskType,
		ProviderID:    provider.ID,
		Model:         "gpt-4",
	}
	require.NoError(t, rrepo.Create(ctx, rule))

	result, err := router.Match(ctx, RouteRequest{
		RepoPath: "org/repo",
		TaskType: "other-task",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestMatch_HighestPriorityWins(t *testing.T) {
	pool := setupTestPool(t)
	prepo := repos.NewProviderRepo(pool)
	rrepo := repos.NewRoutingRuleRepo(pool)
	router := NewRouter(rrepo, prepo)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)

	lowRule := &repos.RoutingRule{
		Name:       "low-priority-" + uuid.NewString()[:8],
		Priority:   1,
		IsActive:   true,
		ProviderID: provider.ID,
		Model:      "gpt-4",
	}
	require.NoError(t, rrepo.Create(ctx, lowRule))

	highRule := &repos.RoutingRule{
		Name:       "high-priority-" + uuid.NewString()[:8],
		Priority:   100,
		IsActive:   true,
		ProviderID: provider.ID,
		Model:      "claude-3",
	}
	require.NoError(t, rrepo.Create(ctx, highRule))

	result, err := router.Match(ctx, RouteRequest{
		RepoPath: "org/repo",
		TaskType: "review",
		UserID:   uuid.New().String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, highRule.ID, result.Rule.ID, "highest priority rule should win")
	assert.Equal(t, "claude-3", result.Rule.Model)
}
