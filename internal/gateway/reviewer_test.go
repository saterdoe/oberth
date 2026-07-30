package gateway

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/pkg/llm"
)

func mockReviewerResponse(result ReviewResult, feedback string) func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
		content := "Result: " + string(result) + "\nFeedback: " + feedback + "\nSummary: test"
		return &llm.ChatResponse{Content: content}, nil
	}
}

func TestExecuteWithReview_Approved(t *testing.T) {
	var genCalls int32

	providers := map[string]llm.Provider{
		"gen-provider": &mockProvider{
			name: "gen-provider",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				atomic.AddInt32(&genCalls, 1)
				return &llm.ChatResponse{
					Content:      "generated output",
					Model:        req.Model,
					InputTokens:  10,
					OutputTokens: 20,
				}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"rev-provider": &mockProvider{
			name:   "rev-provider",
			chatFn: mockReviewerResponse(ReviewApproved, ""),
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	reviewer := NewReviewer(executor)

	step := Step{
		ID:         "step-1",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
		Reviewer: &StepReviewer{
			ProviderID:    "rev-provider",
			Model:         "claude-3",
			MaxIterations: 3,
		},
	}

	generatorStep := Step{
		ID:         "step-1-gen",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
	}

	resp, err := reviewer.ExecuteWithReview(context.Background(), []llm.Message{{Role: "user", Content: "hello"}}, step, generatorStep)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "generated output", resp.Content)
	assert.Equal(t, int32(1), atomic.LoadInt32(&genCalls))
}

func TestExecuteWithReview_ChangesThenApproved(t *testing.T) {
	var genCalls int32
	var revCalls int32

	providers := map[string]llm.Provider{
		"gen-provider": &mockProvider{
			name: "gen-provider",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				genCallsAtomic := atomic.AddInt32(&genCalls, 1)
				content := "generated output"
				if genCallsAtomic > 1 {
					content = "revised output"
				}
				return &llm.ChatResponse{
					Content:      content,
					Model:        req.Model,
					InputTokens:  10,
					OutputTokens: 20,
				}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"rev-provider": &mockProvider{
			name: "rev-provider",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				revCallsAtomic := atomic.AddInt32(&revCalls, 1)
				if revCallsAtomic == 1 {
					return mockReviewerResponse(ReviewChangesRequested, "needs more detail")(ctx, req)
				}
				return mockReviewerResponse(ReviewApproved, "")(ctx, req)
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	reviewer := NewReviewer(executor)

	step := Step{
		ID:         "step-1",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
		Reviewer: &StepReviewer{
			ProviderID:    "rev-provider",
			Model:         "claude-3",
			MaxIterations: 3,
		},
	}

	generatorStep := Step{
		ID:         "step-1-gen",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
	}

	resp, err := reviewer.ExecuteWithReview(context.Background(), []llm.Message{{Role: "user", Content: "hello"}}, step, generatorStep)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "revised output", resp.Content)
	assert.Equal(t, int32(2), atomic.LoadInt32(&genCalls))
	assert.Equal(t, int32(2), atomic.LoadInt32(&revCalls))
}

func TestExecuteWithReview_Rejected(t *testing.T) {
	providers := map[string]llm.Provider{
		"gen-provider": &mockProvider{
			name: "gen-provider",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					Content:      "generated output",
					Model:        req.Model,
					InputTokens:  10,
					OutputTokens: 20,
				}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"rev-provider": &mockProvider{
			name:   "rev-provider",
			chatFn: mockReviewerResponse(ReviewRejected, ""),
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	reviewer := NewReviewer(executor)

	step := Step{
		ID:         "step-1",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
		Reviewer: &StepReviewer{
			ProviderID:    "rev-provider",
			Model:         "claude-3",
			MaxIterations: 3,
		},
	}

	generatorStep := Step{
		ID:         "step-1-gen",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
	}

	resp, err := reviewer.ExecuteWithReview(context.Background(), []llm.Message{{Role: "user", Content: "hello"}}, step, generatorStep)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rejected")
	assert.Nil(t, resp)
}

func TestExecuteWithReview_MaxIterationsExceeded(t *testing.T) {
	var genCalls int32
	var revCalls int32

	maxIterations := 2

	providers := map[string]llm.Provider{
		"gen-provider": &mockProvider{
			name: "gen-provider",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				atomic.AddInt32(&genCalls, 1)
				return &llm.ChatResponse{
					Content:      "generated output",
					Model:        req.Model,
					InputTokens:  10,
					OutputTokens: 20,
				}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"rev-provider": &mockProvider{
			name: "rev-provider",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				atomic.AddInt32(&revCalls, 1)
				return mockReviewerResponse(ReviewChangesRequested, "needs improvement")(ctx, req)
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	reviewer := NewReviewer(executor)

	step := Step{
		ID:         "step-1",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
		Reviewer: &StepReviewer{
			ProviderID:    "rev-provider",
			Model:         "claude-3",
			MaxIterations: maxIterations,
		},
	}

	generatorStep := Step{
		ID:         "step-1-gen",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
	}

	resp, err := reviewer.ExecuteWithReview(context.Background(), []llm.Message{{Role: "user", Content: "hello"}}, step, generatorStep)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations")
	assert.Nil(t, resp)
	assert.Equal(t, int32(maxIterations+1), atomic.LoadInt32(&genCalls))
	assert.Equal(t, int32(maxIterations+1), atomic.LoadInt32(&revCalls))
}

func TestExecuteWithReview_NoReviewer(t *testing.T) {
	var genCalls int32

	providers := map[string]llm.Provider{
		"gen-provider": &mockProvider{
			name: "gen-provider",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				atomic.AddInt32(&genCalls, 1)
				return &llm.ChatResponse{
					Content:      "direct output",
					Model:        req.Model,
					InputTokens:  10,
					OutputTokens: 20,
				}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	reviewer := NewReviewer(executor)

	step := Step{
		ID:         "step-1",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
	}

	generatorStep := Step{
		ID:         "step-1-gen",
		ProviderID: "gen-provider",
		Model:      "gpt-4",
	}

	resp, err := reviewer.ExecuteWithReview(context.Background(), []llm.Message{{Role: "user", Content: "hello"}}, step, generatorStep)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "direct output", resp.Content)
	assert.Equal(t, int32(1), atomic.LoadInt32(&genCalls))
}
