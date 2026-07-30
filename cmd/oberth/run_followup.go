package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type runDetails struct {
	ID             string          `json:"id"`
	TaskID         string          `json:"task_id"`
	SessionID      string          `json:"session_id"`
	State          string          `json:"state"`
	BaseRepository string          `json:"base_repository"`
	BaseCommit     string          `json:"base_commit"`
	WorktreePath   string          `json:"worktree_path"`
	Branch         string          `json:"branch"`
	ResultBundle   json.RawMessage `json:"result_bundle"`
}

func resolveRun(id string) (runDetails, error) {
	if id != "" {
		var run runDetails
		return run, apiUnwrapGET("/runs/"+id, &run)
	}
	var runs []runDetails
	if err := apiUnwrapGET("/runs", &runs); err != nil {
		return runDetails{}, err
	}
	if len(runs) == 0 {
		return runDetails{}, fmt.Errorf("no runs found")
	}
	return runs[0], nil
}

var resumeCmd = &cobra.Command{
	Use:   "resume [run-id]",
	Short: "Resume the latest interrupted or failed run",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		run, err := resolveRun(id)
		if err != nil {
			return err
		}
		if run.State != "failed" && run.State != "interrupted" && run.State != "blocked" {
			return fmt.Errorf("run %s is not resumable from state %s", shortID(run.ID), run.State)
		}
		var accepted runAccepted
		if err := apiUnwrapPOST("/tasks/"+run.TaskID+"/run", `{}`, &accepted); err != nil {
			return err
		}
		fmt.Printf("Resumed as run %s\n", shortID(accepted.RunID))
		return nil
	},
}

var reviewCmd = &cobra.Command{
	Use:   "review [run-id]",
	Short: "Show the reproducible result bundle for a run",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		run, err := resolveRun(id)
		if err != nil {
			return err
		}
		fmt.Print(formatRunReviewText(run))
		return nil
	},
}

var cancelCmd = &cobra.Command{
	Use:   "cancel [run-id]",
	Short: "Cancel the latest or specified run",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		run, err := resolveRun(id)
		if err != nil {
			return err
		}
		var result struct {
			Status string `json:"status"`
		}
		if err := apiUnwrapPOST("/tasks/"+run.TaskID+"/cancel", `{}`, &result); err != nil {
			return err
		}
		fmt.Printf("Run %s: %s\n", shortID(run.ID), result.Status)
		return nil
	},
}

func newOutcomeCommand(use, short, outcome string) *cobra.Command {
	var note string
	command := &cobra.Command{
		Use:   use + " [run-id]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			run, err := resolveRun(id)
			if err != nil {
				return err
			}
			body, _ := json.Marshal(map[string]string{"outcome": outcome, "note": note})
			var result struct {
				Outcome string `json:"outcome"`
			}
			if err := apiUnwrapPOST("/runs/"+run.ID+"/outcome", string(body), &result); err != nil {
				return err
			}
			fmt.Printf("Run %s: %s\n", shortID(run.ID), result.Outcome)
			if outcome == "corrected" {
				var accepted runAccepted
				if err := apiUnwrapPOST("/tasks/"+run.TaskID+"/run", `{}`, &accepted); err != nil {
					return fmt.Errorf("correction recorded, but retry failed: %w", err)
				}
				fmt.Printf("Correction started as run %s\n", shortID(accepted.RunID))
			}
			return nil
		},
	}
	command.Flags().StringVar(&note, "note", "", "record a review note")
	return command
}

var approveCmd = newOutcomeCommand("approve", "Accept and promote a reviewed run", "accepted")
var rejectCmd = newOutcomeCommand("reject", "Reject and discard a reviewed run", "rejected")
var correctCmd = newOutcomeCommand("correct", "Record that a reviewed run needs correction", "corrected")

var diffCmd = &cobra.Command{
	Use:   "diff [run-id]",
	Short: "Show the recorded diff for a run",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		run, err := resolveRun(id)
		if err != nil {
			return err
		}
		var bundle struct {
			Diff json.RawMessage `json:"diff"`
		}
		if err := json.Unmarshal(run.ResultBundle, &bundle); err != nil {
			return fmt.Errorf("invalid result bundle: %w", err)
		}
		if len(bundle.Diff) == 0 || string(bundle.Diff) == "null" {
			return fmt.Errorf("run %s has no recorded diff", shortID(run.ID))
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			fmt.Println(string(bundle.Diff))
			return nil
		}
		formatted, _ := json.MarshalIndent(bundle.Diff, "", "  ")
		fmt.Printf("Run %s diff\n%s\n", shortID(run.ID), formatted)
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export [run-id]",
	Short: "Export a reproducible run bundle as Markdown or JSON",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := ""
		if len(args) == 1 {
			id = args[0]
		}
		run, err := resolveRun(id)
		if err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		if format != "markdown" && format != "json" {
			return fmt.Errorf("format must be markdown or json")
		}
		data, err := apiGET("/runs/" + run.ID + "/export?format=" + format)
		if err != nil {
			return err
		}
		outputPath, _ := cmd.Flags().GetString("out")
		if outputPath == "" {
			_, err = os.Stdout.Write(data)
			return err
		}
		absolute, err := filepath.Abs(outputPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(absolute, data, 0o600); err != nil {
			return fmt.Errorf("writing export: %w", err)
		}
		fmt.Printf("Exported run %s to %s\n", shortID(run.ID), absolute)
		return nil
	},
}

func formatRunReviewText(run runDetails) string {
	formatted, _ := json.MarshalIndent(run.ResultBundle, "", "  ")
	return fmt.Sprintf("Run %s · %s\n%s\n", shortID(run.ID), run.State, formatted)
}

func init() {
	diffCmd.Flags().String("output", "text", "output format: text or json")
	exportCmd.Flags().String("format", "markdown", "export format: markdown or json")
	exportCmd.Flags().String("out", "", "write to a file instead of stdout")
	rootCmd.AddCommand(resumeCmd, reviewCmd, cancelCmd, approveCmd, rejectCmd, correctCmd, diffCmd, exportCmd)
}
