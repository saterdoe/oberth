//go:build integration

package repos

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(s string) *string { return &s }

func TestApprovalGateRepo_Create(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	g := &ApprovalGate{
		Name:            "test-gate",
		Description:     ptr("A test gate"),
		RequireApproval: true,
		Priority:        10,
		IsActive:        true,
	}
	err := repo.Create(ctx, g)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, g.ID)
	assert.False(t, g.CreatedAt.IsZero())
	assert.False(t, g.UpdatedAt.IsZero())
}

func TestApprovalGateRepo_Create_WithAllFields(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, NewProviderRepo(pool))
	maxCost := 50.0
	g := &ApprovalGate{
		Name:            "full-gate",
		Description:     ptr("Full config gate"),
		RepoPattern:     ptr("repo/*"),
		TaskType:        ptr("deploy"),
		ProviderID:      &provider.ID,
		RequireApproval: true,
		RequireReview:   true,
		DenyCloud:       true,
		RequireTests:    true,
		MaxCost:         &maxCost,
		Priority:        100,
		IsActive:        true,
	}
	err := repo.Create(ctx, g)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, g.ID)
	assert.Equal(t, "full-gate", g.Name)
}

func TestApprovalGateRepo_List(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		g := &ApprovalGate{
			Name:     "gate",
			Priority: i,
			IsActive: true,
		}
		require.NoError(t, repo.Create(ctx, g))
	}

	gates, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(gates), 3)
}

func TestApprovalGateRepo_List_ReturnsEmptySliceWhenNone(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	// Delete all gates
	all, err := repo.List(ctx)
	require.NoError(t, err)
	for _, g := range all {
		_ = repo.Delete(ctx, g.ID)
	}

	gates, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, gates)
}

func TestApprovalGateRepo_Match_ByRepoPattern(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	g := &ApprovalGate{
		Name:            "frontend-gate",
		RepoPattern:     ptr("frontend/*"),
		RequireApproval: true,
		Priority:        10,
		IsActive:        true,
	}
	require.NoError(t, repo.Create(ctx, g))

	matched, err := repo.Match(ctx, "frontend/src", "")
	require.NoError(t, err)
	require.NotNil(t, matched)
	assert.Equal(t, "frontend-gate", matched.Name)
}

func TestApprovalGateRepo_Match_ByTaskType(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	g := &ApprovalGate{
		Name:     "deploy-gate",
		TaskType: ptr("deploy"),
		Priority: 10,
		IsActive: true,
	}
	require.NoError(t, repo.Create(ctx, g))

	matched, err := repo.Match(ctx, "", "deploy")
	require.NoError(t, err)
	require.NotNil(t, matched)
	assert.Equal(t, "deploy-gate", matched.Name)
}

func TestApprovalGateRepo_Match_NoMatch(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	matched, err := repo.Match(ctx, "nonexistent", "unknown")
	require.NoError(t, err)
	assert.Nil(t, matched)
}

func TestApprovalGateRepo_Match_IgnoresInactive(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	g := &ApprovalGate{
		Name:     "inactive-gate",
		TaskType: ptr("deploy"),
		Priority: 10,
		IsActive: false,
	}
	require.NoError(t, repo.Create(ctx, g))

	matched, err := repo.Match(ctx, "", "deploy")
	require.NoError(t, err)
	assert.Nil(t, matched)
}

func TestApprovalGateRepo_Match_ReturnsHighestPriority(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	low := &ApprovalGate{Name: "low", TaskType: ptr("deploy"), Priority: 1, IsActive: true}
	high := &ApprovalGate{Name: "high", TaskType: ptr("deploy"), Priority: 100, IsActive: true}
	require.NoError(t, repo.Create(ctx, low))
	require.NoError(t, repo.Create(ctx, high))

	matched, err := repo.Match(ctx, "", "deploy")
	require.NoError(t, err)
	require.NotNil(t, matched)
	assert.Equal(t, "high", matched.Name)
}

func TestApprovalGateRepo_Delete(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	g := &ApprovalGate{Name: "to-delete", Priority: 1, IsActive: true}
	require.NoError(t, repo.Create(ctx, g))

	err := repo.Delete(ctx, g.ID)
	require.NoError(t, err)
}

func TestApprovalGateRepo_Delete_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewApprovalGateRepo(pool)
	ctx := context.Background()

	err := repo.Delete(ctx, uuid.New())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
