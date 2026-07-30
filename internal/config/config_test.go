package config

import "testing"

func TestDefaultReadsLLMAttemptTimeoutFromEnvironment(t *testing.T) {
	t.Setenv("OBERTH_LLM_ATTEMPT_TIMEOUT", "7m30s")
	if got := Default().LLM.AttemptTimeout; got != "7m30s" {
		t.Fatalf("expected configured attempt timeout, got %q", got)
	}
}
