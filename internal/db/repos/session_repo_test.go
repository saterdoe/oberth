//go:build integration

package repos

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db"
)

func TestSessionRepo_CreateAndGetByID(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewSessionRepo(pool)
	ctx := context.Background()

	s := &Session{
		TaskType: "code_review",
		Status:   "active",
	}
	err := repo.Create(ctx, s)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, s.ID)

	got, err := repo.GetByID(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, "code_review", got.TaskType)
	assert.Equal(t, "active", got.Status)
	assert.False(t, got.StartedAt.IsZero())
}

func TestSessionRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewSessionRepo(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestSessionRepo_Update(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewSessionRepo(pool)
	ctx := context.Background()

	s := &Session{
		TaskType: "chat",
		Status:   "active",
	}
	require.NoError(t, repo.Create(ctx, s))

	s.Status = "completed"
	now := time.Now()
	s.EndedAt = &now
	require.NoError(t, repo.Update(ctx, s))

	got, err := repo.GetByID(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.NotNil(t, got.EndedAt)
}

func TestSessionRepo_Update_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewSessionRepo(pool)
	ctx := context.Background()

	err := repo.Update(ctx, &Session{
		ID:       uuid.New(),
		TaskType: "chat",
		Status:   "active",
	})
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestSessionRepo_List(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewSessionRepo(pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		s := &Session{
			TaskType: "testing",
			Status:   "active",
		}
		require.NoError(t, repo.Create(ctx, s))
	}

	sessions, err := repo.List(ctx, SessionFilter{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sessions), 3)
}

func TestSessionRepo_List_FilterByStatus(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewSessionRepo(pool)
	ctx := context.Background()

	status := "completed"
	s := &Session{TaskType: "filter-test", Status: status}
	require.NoError(t, repo.Create(ctx, s))

	s2 := &Session{TaskType: "filter-test", Status: "active"}
	require.NoError(t, repo.Create(ctx, s2))

	sessions, err := repo.List(ctx, SessionFilter{Status: &status})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sessions), 1)
	for _, sess := range sessions {
		assert.Equal(t, status, sess.Status)
	}
}

func TestSessionRepo_List_FilterByDateRange(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewSessionRepo(pool)
	ctx := context.Background()

	s := &Session{TaskType: "date-range-test", Status: "active"}
	require.NoError(t, repo.Create(ctx, s))

	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Hour)
	sessions, err := repo.List(ctx, SessionFilter{Since: &since, Until: &until})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sessions), 1)
}
