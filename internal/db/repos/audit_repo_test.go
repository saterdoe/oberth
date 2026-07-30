//go:build integration

package repos

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditRepo_Create(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewAuditRepo(pool)
	ctx := context.Background()

	entry := &AuditLogEntry{
		Action:  "test_action",
		Actor:   "user:test-user",
		Details: json.RawMessage(`{"key":"value"}`),
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, entry.ID)
	require.False(t, entry.CreatedAt.IsZero())
}

func TestAuditRepo_Create_NilDetails(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewAuditRepo(pool)
	ctx := context.Background()

	entry := &AuditLogEntry{
		Action: "test_no_details",
		Actor:  "agent:test-agent",
	}
	err := repo.Create(ctx, entry)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, entry.ID)
}

func TestAuditRepo_List(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewAuditRepo(pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		entry := &AuditLogEntry{
			Action: "list_test_action",
			Actor:  "user:list-tester",
		}
		require.NoError(t, repo.Create(ctx, entry))
	}

	entries, err := repo.List(ctx, AuditFilter{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 3)
}

func TestAuditRepo_List_FilterByAction(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewAuditRepo(pool)
	ctx := context.Background()

	action := "filterable_action"
	for i := 0; i < 2; i++ {
		entry := &AuditLogEntry{
			Action: action,
			Actor:  "user:filter-tester",
		}
		require.NoError(t, repo.Create(ctx, entry))
	}

	entry := &AuditLogEntry{
		Action: "other_action",
		Actor:  "user:other",
	}
	require.NoError(t, repo.Create(ctx, entry))

	entries, err := repo.List(ctx, AuditFilter{Action: &action})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 2)
	for _, e := range entries {
		assert.Equal(t, action, e.Action)
	}
}

func TestAuditRepo_List_FilterByActor(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewAuditRepo(pool)
	ctx := context.Background()

	actor := "agent:my-bot"
	entry := &AuditLogEntry{
		Action: "bot_action",
		Actor:  actor,
	}
	require.NoError(t, repo.Create(ctx, entry))

	entries, err := repo.List(ctx, AuditFilter{Actor: &actor})
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, actor, entries[0].Actor)
}

func TestAuditRepo_List_Pagination(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewAuditRepo(pool)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		entry := &AuditLogEntry{
			Action: "paginated_action",
			Actor:  "user:paginated",
		}
		require.NoError(t, repo.Create(ctx, entry))
	}

	limit := 2
	entries, err := repo.List(ctx, AuditFilter{Limit: limit})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(entries), limit)
}
