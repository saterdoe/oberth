package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAI implements the Provider interface for OpenAI's API.
type OpenAI struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAI creates a new OpenAI provider.
// If apiKey is empty, it reads from the OBERTH_OPENAI_API_KEY environment variable.
func NewOpenAI(baseURL, apiKey string) *OpenAI {
	return newOpenAI(baseURL, apiKey, EgressPolicy{AllowLoopback: true})
}

func NewRestrictedOpenAI(baseURL, apiKey string) *OpenAI {
	return newOpenAI(baseURL, apiKey, EgressPolicy{})
}

func newOpenAI(baseURL, apiKey string, policy EgressPolicy) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if apiKey == "" {
		apiKey = os.Getenv("OBERTH_OPENAI_API_KEY")
	}
	return &OpenAI{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: newEgressClient(180*time.Second, policy),
	}
}

// Name returns the provider name.
func (p *OpenAI) Name() string {
	return "openai"
}

// Chat sends a chat completion request and returns the response.
func (p *OpenAI) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	httpReq, err := p.buildRequest(ctx, req, false)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("%w: %w", ErrTimeout, err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	return p.parseResponse(resp)
}

// ChatStream sends a chat completion request and returns a streaming response.
func (p *OpenAI) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	httpReq, err := p.buildRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("%w: %w", ErrTimeout, err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, ErrInvalidAPIKey
		}
		return nil, fmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamEvent)
	go p.readStream(ctx, resp.Body, ch)
	return ch, nil
}

type openAIRequest struct {
	Model       string       `json:"model"`
	Messages    []Message    `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
	Tools       []openAITool `json:"tools,omitempty"`
	ToolChoice  any          `json:"tool_choice,omitempty"`
}

type openAITool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type openAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (p *OpenAI) buildRequest(ctx context.Context, req ChatRequest, stream bool) (*http.Request, error) {
	body := openAIRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      stream,
	}
	for _, tool := range req.Tools {
		native := openAITool{Type: "function"}
		native.Function.Name = tool.Name
		native.Function.Description = tool.Description
		native.Function.Parameters = tool.InputSchema
		body.Tools = append(body.Tools, native)
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	return httpReq, nil
}

func (p *OpenAI) parseResponse(resp *http.Response) (*ChatResponse, error) {
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidAPIKey
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var openAIResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	content := openAIResp.Choices[0].Message.Content
	if content == "" && openAIResp.Choices[0].Message.ReasoningContent != "" {
		content = openAIResp.Choices[0].Message.ReasoningContent
	}

	result := &ChatResponse{
		Model:        openAIResp.Model,
		Content:      content,
		InputTokens:  openAIResp.Usage.PromptTokens,
		OutputTokens: openAIResp.Usage.CompletionTokens,
	}
	if calls := openAIResp.Choices[0].Message.ToolCalls; len(calls) > 0 {
		result.ToolCall = &ToolCall{Name: calls[0].Function.Name, Arguments: normalizeToolArguments(calls[0].Function.Arguments)}
	}
	return result, nil
}

func (p *OpenAI) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			ch <- StreamEvent{Done: true}
			return
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- StreamEvent{Error: fmt.Errorf("parse stream event: %w", err)}
			return
		}

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content == "" {
				content = chunk.Choices[0].Delta.ReasoningContent
			}
			if content != "" {
				select {
				case ch <- StreamEvent{Content: content}:
				case <-ctx.Done():
					ch <- StreamEvent{Error: ErrTimeout}
					return
				}
			}

			if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason == "stop" {
				ch <- StreamEvent{Done: true}
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case ch <- StreamEvent{Error: fmt.Errorf("stream read error: %w", err)}:
		case <-ctx.Done():
		}
	}
}
