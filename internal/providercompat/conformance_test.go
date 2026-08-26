package providercompat

import (
	"context"
	"errors"
	"testing"

	"github.com/saterdoe/oberth/pkg/llm"
)

type fixtureProvider struct{ malformed bool }

func (p fixtureProvider) Name() string { return "fixture" }
func (p fixtureProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.malformed {
		return nil, nil
	}
	response := &llm.ChatResponse{Model: req.Model, Content: "OK", InputTokens: 4, OutputTokens: 1}
	if len(req.Tools) > 0 {
		response.Content = ""
		response.ToolCall = &llm.ToolCall{Name: "finish", Arguments: []byte(`{}`)}
	}
	return response, nil
}
func (p fixtureProvider) ChatStream(ctx context.Context, _ llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	events := make(chan llm.StreamEvent, 2)
	events <- llm.StreamEvent{Content: "OK"}
	events <- llm.StreamEvent{Done: true}
	close(events)
	return events, nil
}

func TestHarnessPassesProviderNeutralContract(t *testing.T) {
	report := (Harness{}).Run(t.Context(), fixtureProvider{}, "fixture-model")
	if len(report.Results) != 5 {
		t.Fatalf("expected five results, got %d", len(report.Results))
	}
	for _, result := range report.Results {
		if result.Status != Pass {
			t.Fatalf("%s did not pass: %+v", result.Capability, result)
		}
	}
}

func TestHarnessReportsMalformedResponseActionably(t *testing.T) {
	report := (Harness{}).Run(t.Context(), fixtureProvider{malformed: true}, "broken")
	if report.Results[0].Status != Fail || report.Results[0].Diagnostic == "" {
		t.Fatalf("unexpected malformed result: %+v", report.Results[0])
	}
}

func TestFailureClassifiesTimeout(t *testing.T) {
	result := failure("chat", errors.Join(llm.ErrTimeout, context.DeadlineExceeded))
	if result.Diagnostic != "chat: request deadline exceeded" {
		t.Fatalf("unexpected diagnostic: %s", result.Diagnostic)
	}
}
