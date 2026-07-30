//go:build integration

package repos

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db"
)

func createTestProvider(t *testing.T, repo *ProviderRepo) *Provider {
	t.Helper()
	p := &Provider{
		Name:         "test-provider-" + uuid.NewString()[:8],
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
		Priority:     0,
	}
	require.NoError(t, repo.Create(context.Background(), p))
	return p
}

func TestRoutingRuleRepo_CreateAndGetByID(t *testing.T) {
	pool := setupTestPool(t)
	prepo := NewProviderRepo(pool)
	rrepo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)
	graph := json.RawMessage(`{"steps":["analyze","generate"]}`)

	rule := &RoutingRule{
		Name:           "test-rule-" + uuid.NewString()[:8],
		Description:    "test description",
		Priority:       1,
		IsActive:       true,
		ProviderID:     provider.ID,
		Model:          "gpt-4",
		ExecutionGraph: graph,
	}
	err := rrepo.Create(ctx, rule)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, rule.ID)

	got, err := rrepo.GetByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Equal(t, rule.ID, got.ID)
	assert.Equal(t, rule.Name, got.Name)
	assert.Equal(t, rule.ProviderID, got.ProviderID)
	assert.JSONEq(t, `{"steps":["analyze","generate"]}`, string(got.ExecutionGraph))
}

func TestRoutingRuleRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestRoutingRuleRepo_Update(t *testing.T) {
	pool := setupTestPool(t)
	prepo := NewProviderRepo(pool)
	rrepo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)
	rule := &RoutingRule{
		Name:        "update-rule-" + uuid.NewString()[:8],
		Description: "original",
		Priority:    2,
		IsActive:    true,
		ProviderID:  provider.ID,
		Model:       "claude-3",
	}
	require.NoError(t, rrepo.Create(ctx, rule))

	rule.Description = "updated"
	rule.Priority = 10
	require.NoError(t, rrepo.Update(ctx, rule))

	got, err := rrepo.GetByID(ctx, rule.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Description)
	assert.Equal(t, 10, got.Priority)
}

func TestRoutingRuleRepo_Update_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	prepo := NewProviderRepo(pool)
	repo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)
	err := repo.Update(ctx, &RoutingRule{
		ID:         uuid.New(),
		Name:       "ghost",
		ProviderID: provider.ID,
		Model:      "gpt-4",
	})
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestRoutingRuleRepo_Delete(t *testing.T) {
	pool := setupTestPool(t)
	prepo := NewProviderRepo(pool)
	rrepo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)
	rule := &RoutingRule{
		Name:       "delete-rule-" + uuid.NewString()[:8],
		Priority:   3,
		IsActive:   true,
		ProviderID: provider.ID,
		Model:      "gpt-4",
	}
	require.NoError(t, rrepo.Create(ctx, rule))
	require.NoError(t, rrepo.Delete(ctx, rule.ID))

	_, err := rrepo.GetByID(ctx, rule.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestRoutingRuleRepo_Delete_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	err := repo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestRoutingRuleRepo_List(t *testing.T) {
	pool := setupTestPool(t)
	prepo := NewProviderRepo(pool)
	rrepo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)
	for i := 0; i < 3; i++ {
		rule := &RoutingRule{
			Name:       "list-rule-" + uuid.NewString()[:8],
			Priority:   i,
			IsActive:   true,
			ProviderID: provider.ID,
			Model:      "gpt-4o-mini",
		}
		require.NoError(t, rrepo.Create(ctx, rule))
	}

	rules, err := rrepo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rules), 3)
}

func TestRoutingRuleRepo_Reorder(t *testing.T) {
	pool := setupTestPool(t)
	prepo := NewProviderRepo(pool)
	rrepo := NewRoutingRuleRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)
	rules := make([]RoutingRule, 3)
	for i := range rules {
		rule := &RoutingRule{
			Name:       "reorder-rule-" + uuid.NewString()[:8],
			Priority:   i * 10,
			IsActive:   true,
			ProviderID: provider.ID,
			Model:      "gpt-4",
		}
		require.NoError(t, rrepo.Create(ctx, rule))
		rules[i] = *rule
	}

	ids := []uuid.UUID{rules[2].ID, rules[0].ID, rules[1].ID}
	require.NoError(t, rrepo.Reorder(ctx, ids))

	got, err := rrepo.GetByID(ctx, rules[2].ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Priority)

	got, err = rrepo.GetByID(ctx, rules[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.Priority)

	got, err = rrepo.GetByID(ctx, rules[1].ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.Priority)
}
