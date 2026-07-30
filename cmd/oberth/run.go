package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/tasktype"
	"github.com/spf13/cobra"
)

type runProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type runTask struct {
	ID string `json:"id"`
}

type runAccepted struct {
	RunID     string `json:"run_id"`
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

type streamEvent struct {
	RunID    string          `json:"run_id"`
	Sequence int64           `json:"sequence"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
}

var runCmd = &cobra.Command{
	Use:   "run <intention>",
	Short: "Run an intention in an isolated, governed workspace",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		intention := strings.TrimSpace(strings.Join(args, " "))
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		root, err := resolveGitRoot(cwd)
		if err != nil {
			return err
		}
		baseCommit, err := gitOutput(root, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("resolve base commit: %w", err)
		}
		baseBranch, err := gitOutput(root, "branch", "--show-current")
		if err != nil {
			return fmt.Errorf("resolve base branch: %w", err)
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "text" {
			fmt.Printf("Repository preflight\nRepository: %s\nBase: %s @ %s\nIsolation: a dedicated worktree will be created; this checkout stays unchanged until approval.\n\n", root, branchLabel(baseBranch), shortID(baseCommit))
		}
		projectID, err := ensureCurrentProject(root)
		if err != nil {
			return err
		}
		taskType := inferTaskType(intention)
		body, _ := json.Marshal(map[string]any{
			"repository_id": projectID,
			"title":         inferTitle(intention),
			"description":   intention,
			"task_type":     taskType,
			"constraints":   []string{},
		})
		var task runTask
		if err := apiUnwrapPOST("/tasks", string(body), &task); err != nil {
			return err
		}
		var accepted runAccepted
		if err := apiUnwrapPOST("/tasks/"+task.ID+"/run", `{}`, &accepted); err != nil {
			return err
		}
		var details runDetails
		if err := apiUnwrapGET("/runs/"+accepted.RunID, &details); err != nil {
			return err
		}
		if output == "json" || output == "stream-json" {
			encoded, _ := json.Marshal(map[string]any{
				"run_id": accepted.RunID, "task_id": accepted.TaskID, "session_id": accepted.SessionID,
				"status": accepted.Status, "base_repository": details.BaseRepository, "base_commit": details.BaseCommit,
				"branch": details.Branch, "worktree_path": details.WorktreePath,
			})
			fmt.Println(string(encoded))
			if output == "stream-json" {
				return streamRunEvents(cmd, accepted.RunID)
			}
			return nil
		}
		fmt.Print(formatRunAcceptedText(accepted, details))
		return nil
	},
}

func formatRunAcceptedText(accepted runAccepted, details runDetails) string {
	return fmt.Sprintf(
		"Run %s started\nTask: %s\nSession: %s\nRepository: %s\nBase commit: %s\nWorktree branch: %s\nWorktree: %s\nThe main checkout remains unchanged until approval.\n",
		shortID(accepted.RunID),
		shortID(accepted.TaskID),
		shortID(accepted.SessionID),
		details.BaseRepository,
		shortID(details.BaseCommit),
		details.Branch,
		details.WorktreePath,
	)
}

func resolveGitRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	root, err := gitOutput(absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%s is not inside a Git repository: %w", absolute, err)
	}
	resolved, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func gitOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func branchLabel(branch string) string {
	if strings.TrimSpace(branch) == "" {
		return "detached HEAD"
	}
	return branch
}

func streamRunEvents(cmd *cobra.Command, runID string) error {
	var cursor int64
	for {
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		var replay struct {
			Events []struct {
				Sequence int64           `json:"sequence"`
				Type     string          `json:"type"`
				Payload  json.RawMessage `json:"payload"`
			} `json:"events"`
		}
		if err := apiUnwrapGET(fmt.Sprintf("/runs/%s/events?after=%d", runID, cursor), &replay); err != nil {
			return err
		}
		for _, event := range replay.Events {
			fmt.Println(formatStreamEvent(streamEvent{
				RunID: runID, Sequence: event.Sequence, Type: event.Type, Payload: event.Payload,
			}))
			cursor = event.Sequence
		}
		var state runDetails
		if err := apiUnwrapGET("/runs/"+runID, &state); err != nil {
			return err
		}
		switch state.State {
		case "review", "completed":
			return nil
		case "blocked", "cancelled", "failed", "interrupted":
			return fmt.Errorf("run %s ended in state %s", shortID(runID), state.State)
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func formatStreamEvent(event streamEvent) string {
	encoded, _ := json.Marshal(event)
	return string(encoded)
}

func ensureCurrentProject(root string) (string, error) {
	var projects []runProject
	if err := apiUnwrapGET("/projects", &projects); err != nil {
		return "", err
	}
	for _, project := range projects {
		absolute, _ := filepath.Abs(project.Path)
		if strings.EqualFold(filepath.Clean(absolute), filepath.Clean(root)) {
			return project.ID, nil
		}
	}
	body, _ := json.Marshal(map[string]any{"name": filepath.Base(root), "path": root})
	var project runProject
	if err := apiUnwrapPOST("/projects", string(body), &project); err != nil {
		return "", err
	}
	return project.ID, nil
}

func inferTitle(intention string) string {
	title := strings.Join(strings.Fields(intention), " ")
	runes := []rune(title)
	if len(runes) > 72 {
		title = string(runes[:69]) + "..."
	}
	return title
}

func inferTaskType(intention string) string {
	return tasktype.Infer(intention)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func init() {
	runCmd.Flags().String("output", "text", "output format: text, json, stream-json")
	rootCmd.AddCommand(runCmd)
}
