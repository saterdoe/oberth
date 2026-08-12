package recovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrashCheckpointsCoverCriticalExecutionStages(t *testing.T) {
	events := []Event{
		{1, "run_started", ""},
		{2, "context_compiled", ""},
		{3, "workflow_stage_started", ""},
		{4, "agent_turn", "write:sha256:one"},
		{5, "verification_baseline", "command:sha256:test"},
		{6, "workflow_stage_completed", ""},
		{7, "run_review", ""},
	}
	wantStages := []string{"created", "context", "stage_started", "agent", "verification", "stage_completed", "review"}
	for index, want := range wantStages {
		checkpoint, err := BuildCheckpoint(events[:index+1])
		require.NoError(t, err)
		assert.Equal(t, want, checkpoint.LastStage)
		assert.Equal(t, int64(index+1), checkpoint.LastSequence)
	}
}

func TestReplayNeverRepeatsConfirmedEffects(t *testing.T) {
	checkpoint, err := BuildCheckpoint([]Event{
		{1, "run_started", ""},
		{2, "agent_turn", "file.write:abc"},
		{3, "agent_turn", "command.exec:def"},
	})
	require.NoError(t, err)
	assert.False(t, checkpoint.ShouldExecute("file.write:abc"))
	assert.False(t, checkpoint.ShouldExecute("command.exec:def"))
	assert.True(t, checkpoint.ShouldExecute("file.write:new"))
}

func TestCheckpointRejectsSequenceGapsCausedByDuplicateEvidence(t *testing.T) {
	_, err := BuildCheckpoint([]Event{{1, "run_started", ""}, {1, "agent_turn", "effect"}})
	assert.ErrorContains(t, err, "non-monotonic")
}
