package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeLifecycleClassification(t *testing.T) {
	now := time.Now()
	resources := []WorktreeResource{
		{RunID: "active", State: "running"},
		{RunID: "review", State: "review"},
		{RunID: "recoverable", State: "interrupted"},
		{RunID: "retained", State: "failed", Finished: now.Add(-time.Hour)},
		{RunID: "clean", State: "completed", Finished: now.Add(-72 * time.Hour)},
		{RunID: "dirty", State: "cancelled", Finished: now.Add(-72 * time.Hour), Dirty: true},
		{RunID: "future", State: "new-state"},
	}
	plans := PlanWorktreeLifecycle(resources, now, 24*time.Hour)
	assert.Equal(t, []string{"active", "active", "recoverable", "retained", "prunable", "quarantine", "unknown"}, []string{
		plans[0].Classification, plans[1].Classification, plans[2].Classification, plans[3].Classification,
		plans[4].Classification, plans[5].Classification, plans[6].Classification,
	})
	assert.Equal(t, LifecycleDelete, plans[4].Action)
	assert.Equal(t, LifecycleQuarantine, plans[5].Action)
}

func TestDryRunNeverMutatesAndRecoverableCannotBeForced(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	workspace, err := CreateSessionWorktree(dir, t.TempDir(), "gc-dry-run")
	require.NoError(t, err)
	resource := WorktreeResource{RunID: "run", State: "failed", Finished: time.Now().Add(-72 * time.Hour), Worktree: workspace}
	plan := PlanWorktreeLifecycle([]WorktreeResource{resource}, time.Now(), time.Hour)[0]
	require.NoError(t, ExecuteWorktreePlan(plan, true, t.TempDir()))
	assert.DirExists(t, workspace.Path)
	plan.Classification, plan.Action = "recoverable", LifecycleDelete
	require.Error(t, ExecuteWorktreePlan(plan, false, t.TempDir()))
	assert.DirExists(t, workspace.Path)
}

func TestDirtyTerminalWorktreeIsQuarantinedIdempotently(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	workspace, err := CreateSessionWorktree(dir, t.TempDir(), "gc-quarantine")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(workspace.Path, "residual.txt"), []byte("inspect"), 0600))
	dirty, err := WorktreeDirty(workspace.Path)
	require.NoError(t, err)
	resource := WorktreeResource{RunID: "run-quarantine", State: "failed", Finished: time.Now().Add(-72 * time.Hour), Dirty: dirty, Worktree: workspace}
	plan := PlanWorktreeLifecycle([]WorktreeResource{resource}, time.Now(), time.Hour)[0]
	quarantine := t.TempDir()
	require.NoError(t, ExecuteWorktreePlan(plan, false, quarantine))
	target := filepath.Join(quarantine, resource.RunID)
	assert.DirExists(t, target)
	assert.FileExists(t, filepath.Join(target, ".oberth-quarantine.json"))
	require.NoError(t, ExecuteWorktreePlan(plan, false, quarantine))
}

func TestQuarantineRepairsCrashBetweenMoveAndReport(t *testing.T) {
	dir := initRepo(t)
	writeAndCommit(t, dir, "README.md", "base", "initial")
	workspace, err := CreateSessionWorktree(dir, t.TempDir(), "gc-crash-repair")
	require.NoError(t, err)
	quarantine := t.TempDir()
	target := filepath.Join(quarantine, "run-crash")
	out, err := runCmd(context.Background(), "git", "-C", dir, "worktree", "move", workspace.Path, target)
	require.NoError(t, err, string(out))
	plan := WorktreePlan{
		Resource:       WorktreeResource{RunID: "run-crash", State: "failed", Worktree: workspace},
		Classification: "quarantine", Action: LifecycleQuarantine, Target: workspace.Path,
	}
	require.NoError(t, ExecuteWorktreePlan(plan, false, quarantine))
	assert.FileExists(t, filepath.Join(target, ".oberth-quarantine.json"))
}
