package agentprofile

import (
	"strings"
	"testing"
)

func TestBuildTaskPromptSeparatesTaskDataAndIncludesVersion(t *testing.T) {
	prompt := BuildTaskPrompt("  Fix login  ", "Handle expiry", []byte(`["no schema changes"]`))
	for _, expected := range []string{"Task: Fix login", "Description: Handle expiry", "no schema changes", SingleTaskVersion} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected prompt to contain %q: %s", expected, prompt)
		}
	}
}

func TestSingleTaskProfileKeepsMVPSequentialAndVerified(t *testing.T) {
	for _, expected := range []string{"Do not create subagents", "smallest coherent change", "verification evidence"} {
		if !strings.Contains(SingleTaskSystemPrompt, expected) {
			t.Fatalf("execution contract must contain %q", expected)
		}
	}
}
