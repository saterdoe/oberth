package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [run-id]",
	Short: "Show the latest or specified run",
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
		fmt.Print(formatRunStatusText(run))
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "daemon-status",
	Short: "Show daemon and local subsystem health",
	RunE: func(_ *cobra.Command, _ []string) error {
		data, err := apiGET("/status")
		if err != nil {
			return fmt.Errorf("oberth-server is unavailable: %w", err)
		}
		var envelope struct {
			Data struct {
				Server struct {
					State string `json:"state"`
				} `json:"server"`
				Database struct {
					State string `json:"state"`
				} `json:"database"`
				VectorStore struct {
					State string `json:"state"`
				} `json:"vector_store"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return fmt.Errorf("invalid daemon status response: %w", err)
		}
		fmt.Printf("oberth-server %s (port: %d, database: %s, vector store: %s)\n",
			envelope.Data.Server.State, apiPort, envelope.Data.Database.State, envelope.Data.VectorStore.State)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(daemonStatusCmd)
}

func formatRunStatusText(run runDetails) string {
	var bundle struct {
		Cost               float64 `json:"cost"`
		VerificationStatus any     `json:"verification_status"`
	}
	_ = json.Unmarshal(run.ResultBundle, &bundle)
	next := runNextAction(run.State)
	return fmt.Sprintf("Run %s\nState: %s\nTask: %s\nCost: $%.4f\nVerification: %v\nNext action: %s\n",
		shortID(run.ID), run.State, shortID(run.TaskID), bundle.Cost,
		bundle.VerificationStatus, next)
}

func runNextAction(state string) string {
	switch state {
	case "review":
		return "Inspect `oberth diff`, then run approve, correct or reject."
	case "blocked":
		return "Inspect `oberth review`, address the blocker, then resume."
	case "failed", "interrupted":
		return "Inspect `oberth review`, then run `oberth resume`."
	case "running", "pending":
		return "Wait for events or run `oberth cancel`."
	case "completed":
		return "No action required."
	case "cancelled":
		return "Start a new run when ready."
	default:
		return "Run `oberth review` for evidence."
	}
}
