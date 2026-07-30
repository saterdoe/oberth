//go:build integration

package repos

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db"
)

func TestExecutionLogRepo_Create(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	erepo := NewExecutionLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	el := &ExecutionLog{
		SessionID:   session.ID,
		StepID:      "step-1",
		Model:       "gpt-4",
		Status:      "pending",
		TokensInput: 0,
	}
	err := erepo.Create(ctx, el)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, el.ID)
}

func TestExecutionLogRepo_UpdateStatus(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	erepo := NewExecutionLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	el := &ExecutionLog{
		SessionID: session.ID,
		StepID:    "step-update",
		Model:     "claude-3",
		Status:    "running",
	}
	require.NoError(t, erepo.Create(ctx, el))

	require.NoError(t, erepo.UpdateStatus(ctx, el.ID, "success", nil))

	logs, err := erepo.ListBySession(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "success", logs[0].Status)
	assert.NotNil(t, logs[0].CompletedAt)
}

func TestExecutionLogRepo_UpdateStatus_WithError(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	erepo := NewExecutionLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	el := &ExecutionLog{
		SessionID: session.ID,
		StepID:    "step-error",
		Model:     "gpt-4",
		Status:    "running",
	}
	require.NoError(t, erepo.Create(ctx, el))

	errMsg := "rate limit exceeded"
	require.NoError(t, erepo.UpdateStatus(ctx, el.ID, "failed", &errMsg))

	logs, err := erepo.ListBySession(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "failed", logs[0].Status)
	assert.NotNil(t, logs[0].ErrorMessage)
	assert.Equal(t, errMsg, *logs[0].ErrorMessage)
	assert.NotNil(t, logs[0].CompletedAt)
}

func TestExecutionLogRepo_UpdateStatus_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	erepo := NewExecutionLogRepo(pool)
	ctx := context.Background()

	err := erepo.UpdateStatus(ctx, uuid.New(), "success", nil)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestExecutionLogRepo_ListBySession(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	erepo := NewExecutionLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	for i := 0; i < 3; i++ {
		el := &ExecutionLog{
			SessionID: session.ID,
			StepID:    fmt.Sprintf("step-%d", i),
			Model:     "gpt-4",
			Status:    "pending",
		}
		require.NoError(t, erepo.Create(ctx, el))
	}

	logs, err := erepo.ListBySession(ctx, session.ID)
	require.NoError(t, err)
	assert.Len(t, logs, 3)

	otherSession := createTestSession(t, srepo)
	otherLogs, err := erepo.ListBySession(ctx, otherSession.ID)
	require.NoError(t, err)
	assert.Empty(t, otherLogs)
}

func TestExecutionLogRepo_UpdateStatus_NonTerminal(t *testing.T) {
	pool := setupTestPool(t)
	srepo := NewSessionRepo(pool)
	erepo := NewExecutionLogRepo(pool)
	ctx := context.Background()

	session := createTestSession(t, srepo)
	el := &ExecutionLog{
		SessionID: session.ID,
		StepID:    "step-nonterm",
		Model:     "gpt-4",
		Status:    "pending",
	}
	require.NoError(t, erepo.Create(ctx, el))

	require.NoError(t, erepo.UpdateStatus(ctx, el.ID, "running", nil))

	logs, err := erepo.ListBySession(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", logs[0].Status)
	assert.Nil(t, logs[0].CompletedAt)
}
