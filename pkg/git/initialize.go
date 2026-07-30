package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// InitializeRepository creates a Git repository with an initial empty commit so
// it can immediately be used as the base of an isolated worktree.
func InitializeRepository(ctx context.Context, path string) error {
	run := func(args ...string) error {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	if err := run("init"); err != nil {
		return fmt.Errorf("initialize Git repository: %w", err)
	}
	if err := run("-c", "user.name=oberth", "-c", "user.email=local@oberth", "commit", "--allow-empty", "-m", "chore: initialize project"); err != nil {
		return fmt.Errorf("create initial Git commit: %w", err)
	}
	return nil
}
