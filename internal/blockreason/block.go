package blockreason

import (
	"context"
	"errors"
	"strings"

	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/pkg/llm"
)

type Block struct {
	Code        string `json:"code"`
	Cause       string `json:"cause"`
	NextAction  string `json:"next_action"`
	Recoverable bool   `json:"recoverable"`
}

func Classify(err error) Block {
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return Block{"process_cancelled", err.Error(), "Start a new run when ready.", true}
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, llm.ErrTimeout):
		return Block{"provider_timeout", err.Error(), "Retry, wait for the local model to become available, or select another provider.", true}
	case errors.Is(err, agentruntime.ErrBudgetExceeded):
		return Block{"budget_exhausted", err.Error(), "Increase the run budget or narrow the intention.", true}
	case errors.Is(err, agentruntime.ErrModelFormatExhausted):
		return Block{"model_response_unusable", "The selected model did not produce a usable action after automatic recovery.", "Retry with another local model or simplify the requested change.", true}
	case errors.Is(err, agentruntime.ErrInvalidAction):
		return Block{"invalid_model_format", err.Error(), "Retry with a certified structured-output model.", true}
	case strings.Contains(lower, "approval"):
		return Block{"permission_required", err.Error(), "Review and approve the requested high-risk action.", true}
	case strings.Contains(lower, "not found") || strings.Contains(lower, "no such file"):
		return Block{"file_not_found", err.Error(), "Correct the path or ask the agent to search first.", true}
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "circuit"):
		return Block{"provider_saturated", err.Error(), "Retry after backoff or select another provider.", true}
	case strings.Contains(lower, "providers in fallback chain") || strings.Contains(lower, "provider unavailable"):
		return Block{"provider_unavailable", err.Error(), "Verify the provider or select another model, then retry.", true}
	case strings.Contains(lower, "test") && strings.Contains(lower, "fail"):
		return Block{"tests_failed", err.Error(), "Inspect the verification output and request a correction.", true}
	default:
		return Block{"infrastructure_unavailable", err.Error(), "Inspect daemon and provider health, then retry.", false}
	}
}
