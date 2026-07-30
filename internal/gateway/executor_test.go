package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/pkg/llm"
)

type mockProvider struct {
	name     string
	chatFn   func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error)
}

type activeLocalProvider struct {
	delay time.Duration
}

func (p *activeLocalProvider) Name() string { return "ollama" }
func (p *activeLocalProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.delay):
		return &llm.ChatResponse{Model: req.Model, Content: "local success"}, nil
	}
}
func (p *activeLocalProvider) ChatStream(context.Context, llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, errors.New("not implemented")
}
func (p *activeLocalProvider) ProbeActivity(context.Context, string) llm.Activity {
	return llm.Activity{Reachable: true, Active: true, State: "model_loaded", CheckedAt: time.Now()}
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return m.chatFn(ctx, req)
}

func (m *mockProvider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return m.streamFn(ctx, req)
}

func TestExecuteStep_PrimarySuccess(t *testing.T) {
	providers := map[string]llm.Provider{
		"provider-a": &mockProvider{
			name: "provider-a",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					Model:        req.Model,
					Content:      "success response",
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
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
	}

	resp, err := executor.ExecuteStep(context.Background(), step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "gpt-4", resp.Model)
	assert.Equal(t, "success response", resp.Content)
	assert.Equal(t, 10, resp.InputTokens)
	assert.Equal(t, 20, resp.OutputTokens)
}

func TestExecuteStep_PrimaryTimeout_FallbackSuccess(t *testing.T) {
	providers := map[string]llm.Provider{
		"provider-a": &mockProvider{
			name: "provider-a",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(200 * time.Millisecond):
					return &llm.ChatResponse{Content: "slow response"}, nil
				}
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"provider-b": &mockProvider{
			name: "provider-b",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					Model:        req.Model,
					Content:      "fallback response",
					InputTokens:  5,
					OutputTokens: 15,
				}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 50 * time.Millisecond})
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
		Fallbacks: []Step{
			{
				ID:         "fallback-1",
				ProviderID: "provider-b",
				Model:      "claude-3",
			},
		},
	}

	resp, err := executor.ExecuteStep(context.Background(), step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "claude-3", resp.Model)
	assert.Equal(t, "fallback response", resp.Content)
}

func TestExecuteStep_ExtendsTimeoutForActiveLocalModel(t *testing.T) {
	var decisions []TimeoutDecision
	executor := NewStepExecutor(map[string]llm.Provider{
		"local": &activeLocalProvider{delay: 35 * time.Millisecond},
	}, ExecutorConfig{
		DefaultTimeout: 20 * time.Millisecond,
		OnTimeoutDecision: func(_ context.Context, event TimeoutDecision) {
			decisions = append(decisions, event)
		},
	})
	response, err := executor.ExecuteStep(context.Background(), Step{
		ID: "adaptive-local", ProviderID: "local", Model: "qwen",
	}, []llm.Message{{Role: "user", Content: "work"}})
	require.NoError(t, err)
	require.Equal(t, "local success", response.Content)
	require.Len(t, decisions, 1)
	assert.Equal(t, "extend", decisions[0].Decision)
	assert.Equal(t, "model_loaded", decisions[0].Reason)
}

func TestExecuteStepDoesNotBlindlyExtendCloudTimeout(t *testing.T) {
	provider := &mockProvider{
		name: "openai",
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(40 * time.Millisecond):
				return &llm.ChatResponse{Content: "too late"}, nil
			}
		},
		streamFn: func(context.Context, llm.ChatRequest) (<-chan llm.StreamEvent, error) {
			return nil, errors.New("not implemented")
		},
	}
	var decision TimeoutDecision
	executor := NewStepExecutor(map[string]llm.Provider{"cloud": provider}, ExecutorConfig{
		DefaultTimeout: 15 * time.Millisecond,
		MaxRetries:     -1,
		OnTimeoutDecision: func(_ context.Context, event TimeoutDecision) {
			decision = event
		},
	})
	_, err := executor.ExecuteStep(context.Background(), Step{
		ID: "adaptive-cloud", ProviderID: "cloud", Model: "cloud-model",
	}, []llm.Message{{Role: "user", Content: "work"}})
	require.Error(t, err)
	assert.Equal(t, "timeout", decision.Decision)
	assert.Contains(t, decision.Reason, "no observable progress")
}

func TestExecuteStep_EmitsFallbackEvent(t *testing.T) {
	primaryErr := errors.New("rate limit exceeded")
	providers := map[string]llm.Provider{
		"provider-a": &mockProvider{
			name: "provider-a",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return nil, primaryErr
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"provider-b": &mockProvider{
			name: "provider-b",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{Model: req.Model, Content: "fallback response"}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	var events []FallbackEvent
	executor := NewStepExecutor(providers, ExecutorConfig{
		DefaultTimeout: 5 * time.Second,
		OnFallback: func(ctx context.Context, event FallbackEvent) {
			assert.Equal(t, "session-1", AuditSessionID(ctx))
			events = append(events, event)
		},
	})
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
		Fallbacks: []Step{{
			ID:         "fallback-1",
			ProviderID: "provider-b",
			Model:      "claude-3",
		}},
	}

	resp, err := executor.ExecuteStep(WithAuditSessionID(context.Background(), "session-1"), step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, events, 1)
	assert.Equal(t, "step-1", events[0].StepID)
	assert.Equal(t, "provider-a", events[0].FromProvider)
	assert.Equal(t, "gpt-4", events[0].FromModel)
	assert.Equal(t, "provider-b", events[0].ToProvider)
	assert.Equal(t, "claude-3", events[0].ToModel)
	assert.Equal(t, 1, events[0].Attempt)
	assert.Contains(t, events[0].Error, "rate limit exceeded")
}

func TestExecuteStep_Primary500_FallbackSuccess(t *testing.T) {
	primaryErr := errors.New("500 Internal Server Error")
	providers := map[string]llm.Provider{
		"provider-a": &mockProvider{
			name: "provider-a",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return nil, primaryErr
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"provider-b": &mockProvider{
			name: "provider-b",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					Model:        req.Model,
					Content:      "fallback response",
					InputTokens:  5,
					OutputTokens: 15,
				}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
		Fallbacks: []Step{
			{
				ID:         "fallback-1",
				ProviderID: "provider-b",
				Model:      "claude-3",
			},
		},
	}

	resp, err := executor.ExecuteStep(context.Background(), step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "claude-3", resp.Model)
	assert.Equal(t, "fallback response", resp.Content)
}

func TestExecuteStep_AllFallbacksFail(t *testing.T) {
	primaryErr := errors.New("rate limit exceeded")
	fallbackErr := errors.New("provider unavailable")
	providers := map[string]llm.Provider{
		"provider-a": &mockProvider{
			name: "provider-a",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return nil, primaryErr
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"provider-b": &mockProvider{
			name: "provider-b",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return nil, fallbackErr
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
		Fallbacks: []Step{
			{
				ID:         "fallback-1",
				ProviderID: "provider-b",
				Model:      "claude-3",
			},
		},
	}

	resp, err := executor.ExecuteStep(context.Background(), step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.Error(t, err)
	require.Nil(t, resp)

	var fallbackErrType *FallbackError
	require.True(t, errors.As(err, &fallbackErrType))
	require.Len(t, fallbackErrType.Tried, 2)

	assert.Equal(t, "provider-a", fallbackErrType.Tried[0].Provider)
	assert.Equal(t, "gpt-4", fallbackErrType.Tried[0].Model)
	assert.ErrorIs(t, fallbackErrType.Tried[0].Err, primaryErr)

	assert.Equal(t, "provider-b", fallbackErrType.Tried[1].Provider)
	assert.Equal(t, "claude-3", fallbackErrType.Tried[1].Model)
	assert.ErrorIs(t, fallbackErrType.Tried[1].Err, fallbackErr)
}

func TestExecuteStep_NoFallbacks_PrimaryFails(t *testing.T) {
	primaryErr := errors.New("budget exceeded")
	providers := map[string]llm.Provider{
		"provider-a": &mockProvider{
			name: "provider-a",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return nil, primaryErr
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
	}

	resp, err := executor.ExecuteStep(context.Background(), step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.Error(t, err)
	require.Nil(t, resp)

	var fallbackErrType *FallbackError
	require.True(t, errors.As(err, &fallbackErrType))
	require.Len(t, fallbackErrType.Tried, 1)
	assert.Equal(t, "provider-a", fallbackErrType.Tried[0].Provider)
	assert.Equal(t, "gpt-4", fallbackErrType.Tried[0].Model)
	assert.ErrorIs(t, fallbackErrType.Tried[0].Err, primaryErr)
}

func TestExecuteStep_ContextCancelled(t *testing.T) {
	providers := map[string]llm.Provider{
		"provider-a": &mockProvider{
			name: "provider-a",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(200 * time.Millisecond):
					return &llm.ChatResponse{Content: "too late"}, nil
				}
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
		"provider-b": &mockProvider{
			name: "provider-b",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{Content: "should not reach"}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
		Fallbacks: []Step{
			{
				ID:         "fallback-1",
				ProviderID: "provider-b",
				Model:      "claude-3",
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := executor.ExecuteStep(ctx, step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.Error(t, err)
	require.Nil(t, resp)

	var fallbackErrType *FallbackError
	require.True(t, errors.As(err, &fallbackErrType))
	require.Len(t, fallbackErrType.Tried, 1)
	assert.Equal(t, "provider-a", fallbackErrType.Tried[0].Provider)
	assert.Equal(t, "gpt-4", fallbackErrType.Tried[0].Model)
	assert.ErrorIs(t, fallbackErrType.Tried[0].Err, context.Canceled)
}

func TestExecuteStep_ProviderNotFound(t *testing.T) {
	providers := map[string]llm.Provider{
		"provider-b": &mockProvider{
			name: "provider-b",
			chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{Model: req.Model, Content: "fallback response"}, nil
			},
			streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
				return nil, errors.New("not implemented")
			},
		},
	}

	executor := NewStepExecutor(providers, ExecutorConfig{DefaultTimeout: 5 * time.Second})
	step := Step{
		ID:         "step-1",
		ProviderID: "provider-a",
		Model:      "gpt-4",
		Fallbacks: []Step{
			{
				ID:         "fallback-1",
				ProviderID: "provider-b",
				Model:      "claude-3",
			},
		},
	}

	resp, err := executor.ExecuteStep(context.Background(), step, []llm.Message{
		{Role: "user", Content: "hello"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "claude-3", resp.Model)
	assert.Equal(t, "fallback response", resp.Content)
}

func TestExecuteStepStreamEmitsChunksAndBuildsResponse(t *testing.T) {
	provider := &mockProvider{
		streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent, 3)
			ch <- llm.StreamEvent{Content: "hello "}
			ch <- llm.StreamEvent{Content: "world"}
			ch <- llm.StreamEvent{Done: true}
			close(ch)
			return ch, nil
		},
	}
	executor := NewStepExecutor(map[string]llm.Provider{"local": provider}, ExecutorConfig{})
	var chunks []string
	response, err := executor.ExecuteStepStream(context.Background(), Step{ID: "task", ProviderID: "local", Model: "model"}, []llm.Message{{Role: "user", Content: "go"}}, func(content string) {
		chunks = append(chunks, content)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(chunks, ""); got != "hello world" {
		t.Fatalf("unexpected chunks %q", got)
	}
	if response.Content != "hello world" || response.Model != "model" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestExecuteStepStreamUsesInactivityRatherThanTotalTimeout(t *testing.T) {
	provider := &mockProvider{
		name: "openai",
		streamFn: func(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
			ch := make(chan llm.StreamEvent)
			go func() {
				defer close(ch)
				for _, content := range []string{"still ", "making ", "progress"} {
					select {
					case <-ctx.Done():
						return
					case <-time.After(10 * time.Millisecond):
						ch <- llm.StreamEvent{Content: content}
					}
				}
				ch <- llm.StreamEvent{Done: true}
			}()
			return ch, nil
		},
	}
	executor := NewStepExecutor(map[string]llm.Provider{"cloud": provider}, ExecutorConfig{
		DefaultTimeout: 100 * time.Millisecond,
	})
	response, err := executor.ExecuteStepStream(context.Background(), Step{
		ID: "progressing-stream", ProviderID: "cloud", Model: "model",
	}, []llm.Message{{Role: "user", Content: "go"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, "still making progress", response.Content)
}

func TestExecuteStepResolvesProviderWhenInitialRegistryIsNil(t *testing.T) {
	provider := &mockProvider{
		name: "resolved",
		chatFn: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{Content: "ok"}, nil
		},
	}
	executor := NewStepExecutor(nil, ExecutorConfig{
		ResolveProvider: func(context.Context, string) (llm.Provider, error) {
			return provider, nil
		},
	})

	response, err := executor.ExecuteStep(context.Background(), Step{
		ID: "resolve", ProviderID: "dynamic", Model: "model",
	}, []llm.Message{{Role: "user", Content: "go"}})

	require.NoError(t, err)
	assert.Equal(t, "ok", response.Content)
}
