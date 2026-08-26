package context

import (
	"fmt"
	"strings"
)

const conservativeUnknownContextWindow = 4096

type ModelCapabilityMetadata struct {
	ContextWindow   int    `json:"context_window"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	ToolOverhead    int    `json:"tool_overhead_tokens,omitempty"`
	Source          string `json:"source,omitempty"`
}

type EffectiveModelBudget struct {
	RequestedInputTokens int    `json:"requested_input_tokens"`
	AvailableInputTokens int    `json:"available_input_tokens"`
	ReservedOutputTokens int    `json:"reserved_output_tokens"`
	ToolOverheadTokens   int    `json:"tool_overhead_tokens"`
	SafePromptTokens     int    `json:"safe_prompt_tokens"`
	CapabilitySource     string `json:"capability_source"`
	CapabilityDiagnostic string `json:"capability_diagnostic,omitempty"`
}

func ResolveModelBudget(providerType, model, role string, requestedInput, requestedOutput int, metadata *ModelCapabilityMetadata) EffectiveModelBudget {
	window, source, diagnostic := inferredModelWindow(providerType, model)
	maxOutput, toolOverhead := 0, 256
	if metadata != nil {
		if metadata.ContextWindow >= 1024 && metadata.ContextWindow <= 2_000_000 &&
			metadata.MaxOutputTokens >= 0 && metadata.MaxOutputTokens < metadata.ContextWindow && metadata.ToolOverhead >= 0 {
			window, maxOutput, toolOverhead = metadata.ContextWindow, metadata.MaxOutputTokens, metadata.ToolOverhead
			source = strings.TrimSpace(metadata.Source)
			if source == "" {
				source = "configured"
			}
			diagnostic = ""
		} else {
			window, source = conservativeUnknownContextWindow, "conservative_default"
			diagnostic = "invalid capability metadata; using a conservative 4096-token context window"
		}
	}
	if requestedInput <= 0 {
		requestedInput = window
	}
	if requestedOutput <= 0 {
		requestedOutput = roleOutputReserve(role)
	}
	if maxOutput > 0 && requestedOutput > maxOutput {
		requestedOutput = maxOutput
	}
	if requestedOutput >= window {
		requestedOutput = window / 4
	}
	available := requestedInput
	if available > window {
		available = window
	}
	safe := available - requestedOutput - toolOverhead
	if safe < 256 {
		safe = 256
	}
	return EffectiveModelBudget{
		RequestedInputTokens: requestedInput, AvailableInputTokens: available,
		ReservedOutputTokens: requestedOutput, ToolOverheadTokens: toolOverhead,
		SafePromptTokens: safe, CapabilitySource: source, CapabilityDiagnostic: diagnostic,
	}
}

func inferredModelWindow(providerType, model string) (int, string, string) {
	name := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(name, "qwen2.5") || strings.Contains(name, "qwen3"):
		return 32768, "builtin_model_catalog", ""
	case strings.Contains(name, "gemma"):
		return 8192, "builtin_model_catalog", ""
	case strings.Contains(name, "gpt-4.1") || strings.Contains(name, "gpt-5"):
		return 128000, "builtin_model_catalog", ""
	case strings.Contains(name, "claude"):
		return 200000, "builtin_model_catalog", ""
	case strings.EqualFold(strings.TrimSpace(providerType), "ollama"):
		return 8192, "provider_conservative_default", "unknown Ollama model capability; verify the configured context window"
	default:
		return conservativeUnknownContextWindow, "conservative_default", fmt.Sprintf("unknown capability for %s/%s; configure model metadata before using large prompts", providerType, model)
	}
}

func roleOutputReserve(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "analysis", "analyst":
		return 1200
	case "qa", "review":
		return 1000
	case "development", "implementation":
		return 1500
	default:
		return 800
	}
}
