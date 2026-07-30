package context

import (
	"testing"

	"github.com/saterdoe/oberth/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resumeVault struct{ notes map[string][]vault.Note }

func (v resumeVault) ListNotes(dir string) ([]vault.Note, error) { return v.notes[dir], nil }

func TestBuildResumeContextFindsTaskSummaryAndLinkedDecisions(t *testing.T) {
	source := resumeVault{notes: map[string][]vault.Note{
		"tasks":     {{Path: "tasks/TASK-42", Content: "# Large history\n\n## Summary\nImplement safe login\n\n" + string(make([]byte, 4000)), Metadata: map[string]any{"id": "TASK-42"}}},
		"sessions":  {{Path: "sessions/session-1", Content: "## Summary\nLogin API implemented", Metadata: map[string]any{"task_id": "TASK-42"}}},
		"decisions": {{Path: "decisions/ADR-7", Content: "Use Argon2 for TASK-42", Metadata: map[string]any{"type": "decision"}}},
	}}

	result, err := BuildResumeContext(source, "TASK-42", 800)
	require.NoError(t, err)
	assert.Contains(t, result.Summary, "Implement safe login")
	assert.Len(t, result.Decisions, 1)
	assert.Equal(t, "decisions/ADR-7", result.Decisions[0].Path)
	assert.LessOrEqual(t, result.Characters, 800)
	assert.NotContains(t, result.Summary, string(make([]byte, 4000)))
}
