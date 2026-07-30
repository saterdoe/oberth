package api

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/reasoning"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
)

const resultBundleSchemaVersion = "1"

// ResultBundleV1 is the stable, exportable evidence contract for a run.
// Additive fields are allowed in v1; incompatible changes require a new version.
type ResultBundleV1 struct {
	SchemaVersion      string            `json:"schema_version"`
	RunID              uuid.UUID         `json:"run_id"`
	TaskID             uuid.UUID         `json:"task_id"`
	SessionID          uuid.UUID         `json:"session_id"`
	BaseCommit         string            `json:"base_commit"`
	Worktree           string            `json:"worktree"`
	Branch             string            `json:"branch"`
	Diff               []gitpkg.DiffFile `json:"diff"`
	DiffHash           string            `json:"diff_hash"`
	Context            json.RawMessage   `json:"context"`
	ContextHash        string            `json:"context_hash"`
	TokensInput        int               `json:"tokens_input"`
	TokensOutput       int               `json:"tokens_output"`
	Cost               float64           `json:"cost"`
	Warnings           []string          `json:"warnings"`
	Commands           any               `json:"commands,omitempty"`
	VerificationStatus any               `json:"verification_status,omitempty"`
	Runtime            map[string]any    `json:"runtime"`
	Environment        EnvironmentV1     `json:"environment"`
	Reasoning          *reasoning.CaseV1 `json:"reasoning,omitempty"`
	Outcome            string            `json:"outcome,omitempty"`
}

type EnvironmentV1 struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
}
