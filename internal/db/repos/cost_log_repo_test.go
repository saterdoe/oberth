//go:build integration

package repos

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestSession(t *testing.T, repo *SessionRepo) *Session {
	t.Helper()
	ctx := context.Background()
	s := &Session{
		TaskType: "cost-test",
		Status:   "completed",
	}
	require.NoError(t, repo.Create(ctx, s))
	return s
}

func TestCostLogRepo_Create(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	crepo := NewCostLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	cl := &CostLog{
		SessionID:    session.ID,
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    0.001,
		CostOutput:   0.002,
		CostTotal:    0.003,
		CacheHit:     false,
	}
	err := crepo.Create(ctx, cl)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, cl.ID)
	require.False(t, cl.CreatedAt.IsZero())
}

func TestCostLogRepo_List(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	crepo := NewCostLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	for i := 0; i < 3; i++ {
		cl := &CostLog{
			SessionID:    session.ID,
			Model:        "gpt-4o-mini",
			TokensInput:  10,
			TokensOutput: 5,
			CostInput:    0.0001,
			CostOutput:   0.0002,
			CostTotal:    0.0003,
		}
		require.NoError(t, crepo.Create(ctx, cl))
	}

	logs, err := crepo.List(ctx, CostLogFilter{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(logs), 3)
}

func TestCostLogRepo_List_Pagination(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	crepo := NewCostLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	for i := 0; i < 5; i++ {
		cl := &CostLog{
			SessionID:    session.ID,
			Model:        "gpt-4",
			TokensInput:  10,
			TokensOutput: 5,
			CostInput:    0.0001,
			CostOutput:   0.0002,
			CostTotal:    0.0003,
		}
		require.NoError(t, crepo.Create(ctx, cl))
	}

	limit := 2
	logs, err := crepo.List(ctx, CostLogFilter{Limit: limit})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(logs), limit)
}

func TestCostLogRepo_GetSummary(t *testing.T) {
	pool := setupTestPool(t)
	prepo := NewProviderRepo(pool)
	srepo := NewSessionRepo(pool)
	crepo := NewCostLogRepo(pool)
	ctx := context.Background()

	provider := createTestProvider(t, prepo)
	session := createTestSession(t, srepo)

	for i := 0; i < 3; i++ {
		cl := &CostLog{
			SessionID:    session.ID,
			ProviderID:   &provider.ID,
			Model:        "gpt-4",
			TokensInput:  100,
			TokensOutput: 50,
			CostInput:    0.001,
			CostOutput:   0.002,
			CostTotal:    0.003,
		}
		require.NoError(t, crepo.Create(ctx, cl))
	}

	since := time.Now().Add(-1 * time.Hour)
	summary, err := crepo.GetSummary(ctx, since)
	require.NoError(t, err)
	assert.InDelta(t, 0.009, summary.TotalCost, 0.0001)
	assert.Equal(t, int64(450), summary.TotalTokens)
	assert.NotEmpty(t, summary.ByProvider)
	assert.InDelta(t, 0.009, summary.ByProvider[provider.ID.String()], 0.0001)
}

func TestCostLogRepo_GetSummary_Empty(t *testing.T) {
	pool := setupTestPool(t)
	crepo := NewCostLogRepo(pool)
	ctx := context.Background()

	since := time.Now().Add(1 * time.Hour)
	summary, err := crepo.GetSummary(ctx, since)
	require.NoError(t, err)
	assert.InDelta(t, 0, summary.TotalCost, 0.0001)
	assert.Equal(t, int64(0), summary.TotalTokens)
	assert.Empty(t, summary.ByProvider)
}
