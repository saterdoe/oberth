package recovery

import (
	"fmt"
	"sort"
)

type Event struct {
	Sequence int64
	Type     string
	EffectID string
}

type Checkpoint struct {
	LastSequence     int64
	LastStage        string
	ConfirmedEffects map[string]struct{}
}

var stages = map[string]string{
	"run_started":              "created",
	"context_compiled":         "context",
	"workflow_stage_started":   "stage_started",
	"agent_turn":               "agent",
	"verification_baseline":    "verification",
	"workflow_stage_completed": "stage_completed",
	"run_review":               "review",
}

// BuildCheckpoint derives replay state only from the ordered durable event log.
func BuildCheckpoint(events []Event) (Checkpoint, error) {
	ordered := append([]Event(nil), events...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Sequence < ordered[j].Sequence })
	checkpoint := Checkpoint{ConfirmedEffects: map[string]struct{}{}}
	for _, event := range ordered {
		if event.Sequence <= checkpoint.LastSequence {
			return Checkpoint{}, fmt.Errorf("run event sequence is duplicated or non-monotonic at %d", event.Sequence)
		}
		checkpoint.LastSequence = event.Sequence
		if stage := stages[event.Type]; stage != "" {
			checkpoint.LastStage = stage
		}
		if event.EffectID != "" {
			checkpoint.ConfirmedEffects[event.EffectID] = struct{}{}
		}
	}
	return checkpoint, nil
}

// ShouldExecute is false for an effect already confirmed in durable evidence.
func (c Checkpoint) ShouldExecute(effectID string) bool {
	if effectID == "" {
		return true
	}
	_, confirmed := c.ConfirmedEffects[effectID]
	return !confirmed
}
