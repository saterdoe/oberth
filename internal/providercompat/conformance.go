package providercompat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/saterdoe/oberth/pkg/llm"
)

type Status string

const (
	Pass    Status = "pass"
	Partial Status = "partial"
	Fail    Status = "fail"
)

type Result struct {
	Capability string `json:"capability"`
	Status     Status `json:"status"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

type Report struct {
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Results  []Result `json:"results"`
}

type Harness struct {
	Timeout time.Duration
}

// Run exercises the provider-neutral contract without credentials or network
// assumptions. Callers may supply a real provider or a deterministic fixture.
func (h Harness) Run(ctx context.Context, provider llm.Provider, model string) Report {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	report := Report{Provider: provider.Name(), Model: model}
	request := llm.ChatRequest{Model: model, Messages: []llm.Message{{Role: "user", Content: "Reply with OK."}}, MaxTokens: 16}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	response, err := provider.Chat(callCtx, request)
	cancel()
	report.Results = append(report.Results, responseResult("chat", response, err))

	toolRequest := request
	toolRequest.Tools = []llm.ToolDefinition{{Name: "finish", Description: "finish the check", InputSchema: []byte(`{"type":"object"}`)}}
	toolRequest.ToolChoice = "auto"
	callCtx, cancel = context.WithTimeout(ctx, timeout)
	toolResponse, toolErr := provider.Chat(callCtx, toolRequest)
	cancel()
	toolResult := responseResult("tool_calling", toolResponse, toolErr)
	if toolResult.Status == Pass && toolResponse.ToolCall == nil {
		toolResult.Status = Partial
		toolResult.Diagnostic = "provider returned text but no tool call; typed actions are not certified for this model"
	}
	report.Results = append(report.Results, toolResult)

	streamCtx, streamCancel := context.WithTimeout(ctx, timeout)
	stream, streamErr := provider.ChatStream(streamCtx, request)
	streamResult := Result{Capability: "streaming", Status: Pass}
	if streamErr != nil {
		streamResult = failure("streaming", streamErr)
	} else {
		sawContent, sawDone := false, false
		for event := range stream {
			if event.Error != nil {
				streamResult = failure("streaming", event.Error)
				break
			}
			sawContent = sawContent || event.Content != ""
			sawDone = sawDone || event.Done
		}
		if streamResult.Status == Pass && (!sawContent || !sawDone) {
			streamResult = Result{Capability: "streaming", Status: Partial, Diagnostic: "stream ended without both content and a terminal event"}
		}
	}
	streamCancel()
	report.Results = append(report.Results, streamResult)

	cancelledCtx, cancelNow := context.WithCancel(ctx)
	cancelNow()
	_, cancelErr := provider.Chat(cancelledCtx, request)
	if cancelErr == nil {
		report.Results = append(report.Results, Result{Capability: "cancellation", Status: Fail, Diagnostic: "provider accepted an already-cancelled request"})
	} else {
		report.Results = append(report.Results, Result{Capability: "cancellation", Status: Pass})
	}

	deadlineCtx, deadlineCancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer deadlineCancel()
	_, deadlineErr := provider.Chat(deadlineCtx, request)
	if deadlineErr == nil {
		report.Results = append(report.Results, Result{Capability: "timeout", Status: Fail, Diagnostic: "provider ignored an expired deadline"})
	} else {
		report.Results = append(report.Results, Result{Capability: "timeout", Status: Pass})
	}
	return report
}

func responseResult(capability string, response *llm.ChatResponse, err error) Result {
	if err != nil {
		return failure(capability, err)
	}
	if response == nil {
		return Result{Capability: capability, Status: Fail, Diagnostic: "provider returned a nil response without an error"}
	}
	if strings.TrimSpace(response.Content) == "" && response.ToolCall == nil {
		return Result{Capability: capability, Status: Fail, Diagnostic: "provider response contained neither text nor a tool call"}
	}
	if response.InputTokens <= 0 || response.OutputTokens <= 0 {
		return Result{Capability: capability, Status: Partial, Diagnostic: "response omitted usable token accounting"}
	}
	return Result{Capability: capability, Status: Pass}
}

func failure(capability string, err error) Result {
	diagnostic := err.Error()
	if errors.Is(err, context.Canceled) {
		diagnostic = "request cancelled"
	} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, llm.ErrTimeout) {
		diagnostic = "request deadline exceeded"
	}
	return Result{Capability: capability, Status: Fail, Diagnostic: fmt.Sprintf("%s: %s", capability, diagnostic)}
}
