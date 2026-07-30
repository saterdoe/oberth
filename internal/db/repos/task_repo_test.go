//go:build integration

package repos

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/saterdoe/oberth/internal/db"
	"github.com/stretchr/testify/require"
)

func TestTaskRepoLifecycle(t *testing.T) {
	r := NewTaskRepo(setupTestPool(t))
	ctx := context.Background()
	task := &Task{Title: "Fix login", Description: "Repair validation", TaskType: "bug_fix", Constraints: json.RawMessage(`["no schema changes"]`), Status: "pending"}
	require.NoError(t, r.Create(ctx, task))
	got, err := r.GetByID(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "Fix login", got.Title)
	require.NoError(t, r.SetStatus(ctx, task.ID, []string{"pending"}, "running"))
	require.ErrorIs(t, r.SetStatus(ctx, task.ID, []string{"pending"}, "running"), db.ErrConflict)
	list, err := r.List(ctx, TaskFilter{Status: "running"})
	require.NoError(t, err)
	require.NotEmpty(t, list)
}
