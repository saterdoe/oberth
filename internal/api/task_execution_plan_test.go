package api

import (
	"encoding/json"
	"testing"
)

func TestParseTaskExecutionPlan(t *testing.T) {
	raw := json.RawMessage(`{"execution_plan":[
		{"id":"analysis","role":"analysis","provider_id":"local","model":"qwen"},
		{"id":"development","role":"development","provider_id":"cloud","model":"coder"}
	]}`)
	stages := parseTaskExecutionPlan(raw)
	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}
	if primary := primaryExecutionStage(stages); primary == nil || primary.ID != "development" {
		t.Fatalf("expected development as primary stage, got %#v", primary)
	}
}

func TestParseTaskExecutionPlanKeepsLegacyConstraintsCompatible(t *testing.T) {
	if stages := parseTaskExecutionPlan(json.RawMessage(`["do not edit vendor"]`)); len(stages) != 0 {
		t.Fatalf("legacy constraints must not create stages: %#v", stages)
	}
}

func TestValidateTaskExecutionPlanRejectsIncompleteStageInsteadOfFallingBack(t *testing.T) {
	raw := json.RawMessage(`{"execution_plan":[{"id":"development","role":"development","provider_id":"local"}]}`)
	if err := validateTaskExecutionPlan(raw); err == nil {
		t.Fatal("expected an incomplete explicit plan to be rejected")
	}
}

func TestValidateTaskExecutionPlanAcceptsSingleExplicitModel(t *testing.T) {
	raw := json.RawMessage(`{"execution_plan":[{"id":"development","role":"development","provider_id":"local","model":"qwen"}]}`)
	if err := validateTaskExecutionPlan(raw); err != nil {
		t.Fatalf("expected valid single-stage plan: %v", err)
	}
}
