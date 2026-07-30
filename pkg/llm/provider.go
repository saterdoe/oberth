package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Provider defines the interface for LLM providers.
type Provider interface {
	// Chat sends a chat completion request and returns the response.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// ChatStream sends a chat completion request and returns a streaming response.
	ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)

	// Name returns the provider name (e.g., "openai", "anthropic").
	Name() string
}

// Activity reports whether a provider can prove that a request may still be
// making progress. Local runtimes can inspect their own scheduler; cloud
// providers generally cannot and should rely on bounded latency history.
type Activity struct {
	Reachable bool
	Active    bool
	State     string
	CheckedAt time.Time
}

type ActivityProber interface {
	ProbeActivity(ctx context.Context, model string) Activity
}

// Message represents a single message in a chat conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func normalizeToolArguments(raw json.RawMessage) json.RawMessage {
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		return json.RawMessage(encoded)
	}
	return raw
}

// ChatResponse represents a chat completion response.
type ChatResponse struct {
	Model        string    `json:"model"`
	Content      string    `json:"content"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	ToolCall     *ToolCall `json:"tool_call,omitempty"`
}

// StreamEvent represents a single event from a streaming response.
type StreamEvent struct {
	Content string
	Done    bool
	Error   error
}
