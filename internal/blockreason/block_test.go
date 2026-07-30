package blockreason

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/pkg/llm"
)

func TestClassifyBudget(t *testing.T) {
	if got := Classify(agentruntime.ErrBudgetExceeded); got.Code != "budget_exhausted" || !got.Recoverable {
		t.Fatalf("unexpected block: %+v", got)
	}
}

func TestClassifyTypedTimeoutAsRecoverable(t *testing.T) {
	for _, err := range []error{
		context.DeadlineExceeded,
		fmt.Errorf("provider stalled: %w", llm.ErrTimeout),
		&gateway.FallbackError{Tried: []gateway.Attempt{{Provider: "ollama", Model: "local", Err: llm.ErrTimeout}}},
	} {
		got := Classify(err)
		if got.Code != "provider_timeout" || !got.Recoverable {
			t.Fatalf("unexpected timeout block for %v: %+v", err, got)
		}
	}
}

func TestClassifyProviderFailureAsRecoverable(t *testing.T) {
	got := Classify(errors.New("all providers in fallback chain failed: provider unavailable"))
	if got.Code != "provider_unavailable" || !got.Recoverable {
		t.Fatalf("unexpected block: %+v", got)
	}
}
