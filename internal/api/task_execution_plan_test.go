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

func TestParseTaskExecutionPlanKeepsCapabilitySnapshot(t *testing.T) {
	raw := json.RawMessage(`{"execution_plan":[{"id":"qa","role":"qa","provider_id":"local","model":"qwen","capabilities":{"context_window":8192,"max_output_tokens":512,"tool_overhead_tokens":128,"source":"probe"}}]}`)
	stage := parseTaskExecutionPlan(raw)[0]
	if stage.Capabilities == nil || stage.Capabilities.ContextWindow != 8192 || stage.Capabilities.Source != "probe" {
		t.Fatalf("capability snapshot lost: %+v", stage)
	}
}

func TestFitStagePromptReportsDeterministicReduction(t *testing.T) {
	prompt := "task\n\n## Traced repository context\n" + string(make([]byte, 8000))
	first, dropped := fitStagePrompt(prompt, "system", 300)
	second, droppedAgain := fitStagePrompt(prompt, "system", 300)
	if first != second || dropped <= 0 || dropped != droppedAgain {
		t.Fatalf("reduction is not deterministic: %d/%d", dropped, droppedAgain)
	}
}
