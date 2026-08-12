package git

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WorktreeResource struct {
	Worktree SessionWorktree
	RunID    string
	State    string
	Outcome  string
	Finished time.Time
	Dirty    bool
}

type LifecycleAction string

const (
	LifecycleKeep       LifecycleAction = "keep"
	LifecycleDelete     LifecycleAction = "delete"
	LifecycleQuarantine LifecycleAction = "quarantine"
)

type WorktreePlan struct {
	Resource       WorktreeResource `json:"resource"`
	Classification string           `json:"classification"`
	Action         LifecycleAction  `json:"action"`
	Reason         string           `json:"reason"`
	Target         string           `json:"target"`
}

func PlanWorktreeLifecycle(resources []WorktreeResource, now time.Time, retention time.Duration) []WorktreePlan {
	plans := make([]WorktreePlan, 0, len(resources))
	for _, resource := range resources {
		plan := WorktreePlan{Resource: resource, Action: LifecycleKeep, Target: resource.Worktree.Path}
		switch resource.State {
		case "running", "review":
			plan.Classification, plan.Reason = "active", "run is still active or awaiting review"
		case "blocked", "interrupted":
			plan.Classification, plan.Reason = "recoverable", "run may be resumed and its artifacts must survive"
		case "completed", "cancelled", "failed":
			if resource.Finished.IsZero() || now.Sub(resource.Finished) < retention {
				plan.Classification, plan.Reason = "retained", "terminal worktree is inside the retention window"
			} else if resource.Dirty {
				plan.Classification, plan.Action = "quarantine", LifecycleQuarantine
				plan.Reason = "terminal worktree contains residual changes requiring inspection"
			} else {
				plan.Classification, plan.Action = "prunable", LifecycleDelete
				plan.Reason = "terminal clean worktree exceeded its retention window"
			}
		default:
			plan.Classification, plan.Reason = "unknown", "unrecognized state is preserved fail-closed"
		}
		plans = append(plans, plan)
	}
	return plans
}

func WorktreeDirty(path string) (bool, error) {
	status, err := runCmd(context.Background(), "git", "-C", path, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(status)) != "", nil
}

// ExecuteWorktreePlan applies one already-classified action. Dry-run is a
// strict no-op; callers still receive the complete plan as their report.
func ExecuteWorktreePlan(plan WorktreePlan, dryRun bool, quarantineRoot string) error {
	if dryRun || plan.Action == LifecycleKeep {
		return nil
	}
	if plan.Classification == "active" || plan.Classification == "recoverable" {
		return errors.New("refusing to remove active or recoverable worktree")
	}
	if plan.Action == LifecycleDelete {
		if _, err := os.Stat(plan.Resource.Worktree.Path); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return CleanupSessionWorktree(plan.Resource.Worktree, false)
	}
	if plan.Action != LifecycleQuarantine {
		return fmt.Errorf("unknown lifecycle action %q", plan.Action)
	}
	if quarantineRoot == "" {
		return errors.New("quarantine root is required")
	}
	target := filepath.Join(quarantineRoot, plan.Resource.RunID)
	if _, err := os.Stat(target); err == nil {
		return writeQuarantineReport(target, plan)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(plan.Resource.Worktree.Path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := os.MkdirAll(quarantineRoot, 0700); err != nil {
		return err
	}
	if _, err := runCmd(context.Background(), "git", "-C", plan.Resource.Worktree.Repository,
		"worktree", "move", plan.Resource.Worktree.Path, target); err != nil {
		return fmt.Errorf("quarantine worktree: %w", err)
	}
	return writeQuarantineReport(target, plan)
}

func writeQuarantineReport(target string, plan WorktreePlan) error {
	report, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, ".oberth-quarantine.json"), report, 0600)
}
