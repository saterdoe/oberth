package api

import (
	"encoding/json"
	"testing"

	"github.com/saterdoe/oberth/internal/agentruntime"
)

func TestFinishAfterSuccessfulVerification(t *testing.T) {
	observation, _ := json.Marshal(agentruntime.Observation{
		SchemaVersion: "1",
		Tool:          "command",
		Status:        "ok",
	})
	summary, ok := finishAfterSuccessfulVerification([]agentruntime.Message{
		{Role: "user", Content: "Tool observation:\n" + string(observation)},
	}, "All tests passed.")
	if !ok || summary != "All tests passed." {
		t.Fatalf("got (%q, %v)", summary, ok)
	}
}

func TestFinishAfterFailedVerificationIsRejected(t *testing.T) {
	observation, _ := json.Marshal(agentruntime.Observation{
		SchemaVersion: "1",
		Tool:          "command",
		Status:        "failed",
	})
	if _, ok := finishAfterSuccessfulVerification([]agentruntime.Message{
		{Role: "user", Content: "Tool observation:\n" + string(observation)},
	}, "Done."); ok {
		t.Fatal("failed verification must not synthesize finish")
	}
}
