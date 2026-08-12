package api

import (
	"context"
	"testing"
)

func TestMandatoryAuditFailsClosedWithoutRepository(t *testing.T) {
	server := &Server{}
	err := server.writeAudit(context.Background(), nil, "sensitive_effect", "user:local", map[string]any{
		"target": "repository", "decision": "approved",
	})
	if err == nil {
		t.Fatal("mandatory audit unexpectedly succeeded without durable storage")
	}
}

func TestAuditDetailSelectsExplicitTargetAndDecision(t *testing.T) {
	details := map[string]any{"repository": "C:/repo", "outcome": "accepted"}
	if got := auditDetail(details, "target", "repository"); got != "C:/repo" {
		t.Fatalf("target = %q", got)
	}
	if got := auditDetail(details, "decision", "outcome"); got != "accepted" {
		t.Fatalf("decision = %q", got)
	}
}
