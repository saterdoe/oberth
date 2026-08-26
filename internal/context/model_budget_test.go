package context

import "testing"

func TestResolveModelBudgetUsesDifferentRoleReserves(t *testing.T) {
	analysis := ResolveModelBudget("ollama", "qwen2.5-coder:7b", "analysis", 12000, 0, nil)
	implementation := ResolveModelBudget("ollama", "qwen2.5-coder:7b", "implementation", 12000, 0, nil)
	if analysis.ReservedOutputTokens == implementation.ReservedOutputTokens {
		t.Fatal("roles must retain independent output reserves")
	}
}

func TestResolveModelBudgetFailsClosedForMalformedMetadata(t *testing.T) {
	budget := ResolveModelBudget("custom", "unknown", "review", 64000, 2000, &ModelCapabilityMetadata{ContextWindow: -1, Source: "endpoint"})
	if budget.AvailableInputTokens != conservativeUnknownContextWindow || budget.CapabilitySource != "conservative_default" || budget.CapabilityDiagnostic == "" {
		t.Fatalf("unexpected conservative budget: %+v", budget)
	}
}

func TestResolveModelBudgetHonorsValidMetadataAndAllOverhead(t *testing.T) {
	budget := ResolveModelBudget("custom", "private", "qa", 64000, 4000, &ModelCapabilityMetadata{ContextWindow: 16000, MaxOutputTokens: 2000, ToolOverhead: 500, Source: "endpoint_probe"})
	if budget.AvailableInputTokens != 16000 || budget.ReservedOutputTokens != 2000 || budget.SafePromptTokens != 13500 {
		t.Fatalf("unexpected budget: %+v", budget)
	}
}

func TestResolveModelBudgetUsesConservativeUnknownOpenAICompatibleLimit(t *testing.T) {
	budget := ResolveModelBudget("custom", "vendor-private-model", "analysis", 64000, 0, nil)
	if budget.AvailableInputTokens != 4096 || budget.CapabilityDiagnostic == "" {
		t.Fatalf("unknown endpoint was not bounded: %+v", budget)
	}
}

func TestResolveModelBudgetUsesKnownOllamaCatalogEntry(t *testing.T) {
	budget := ResolveModelBudget("ollama", "qwen2.5-coder:7b", "development", 64000, 0, nil)
	if budget.AvailableInputTokens != 32768 || budget.CapabilitySource != "builtin_model_catalog" {
		t.Fatalf("known Ollama model not resolved: %+v", budget)
	}
}
