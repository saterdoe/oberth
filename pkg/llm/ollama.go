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
	"strings"
	"time"
)

// Ollama implements the Provider interface for Ollama's OpenAI-compatible API.
type Ollama struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllama creates a new Ollama provider.
// If baseURL is empty, it defaults to http://localhost:11434/v1.
func NewOllama(baseURL string) *Ollama {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	return &Ollama{
		baseURL: strings.TrimRight(baseURL, "/"),
		// The executor owns request deadlines. A fixed client timeout incorrectly
		// kills healthy local generations while Ollama is still working.
		httpClient: &http.Client{},
	}
}

// Name returns the provider name.
func (p *Ollama) Name() string {
	return "ollama"
}

func (p *Ollama) ProbeActivity(ctx context.Context, model string) Activity {
	activity := Activity{CheckedAt: time.Now()}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	root := strings.TrimSuffix(p.baseURL, "/v1")
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, root+"/api/ps", nil)
	if err != nil {
		activity.State = "probe_error"
		return activity
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		activity.State = "unreachable"
		return activity
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		activity.State = fmt.Sprintf("http_%d", resp.StatusCode)
		return activity
	}
	activity.Reachable = true
	var payload struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		activity.State = "reachable"
		return activity
	}
	wanted := strings.TrimSpace(model)
	for _, loaded := range payload.Models {
		if loaded.Name == wanted || loaded.Model == wanted ||
			strings.TrimSuffix(loaded.Name, ":latest") == strings.TrimSuffix(wanted, ":latest") {
			activity.Active = true
			activity.State = "model_loaded"
			return activity
		}
	}
	// A reachable Ollama daemon may be loading the requested model while /api/ps
	// is still empty. The executor grants one bounded extension for that state.
	activity.State = "daemon_reachable_model_loading"
	return activity
}

// Chat sends a chat completion request and returns the response.
func (p *Ollama) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
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
func (p *Ollama) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	httpReq, err := p.buildRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
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

type ollamaRequest struct {
	Model       string       `json:"model"`
	Messages    []Message    `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
	Tools       []openAITool `json:"tools,omitempty"`
	ToolChoice  any          `json:"tool_choice,omitempty"`
}

type ollamaResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
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

type ollamaStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (p *Ollama) buildRequest(ctx context.Context, req ChatRequest, stream bool) (*http.Request, error) {
	body := ollamaRequest{
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

	return httpReq, nil
}

func (p *Ollama) parseResponse(resp *http.Response) (*ChatResponse, error) {
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidAPIKey
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(ollamaResp.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	result := &ChatResponse{
		Model:        ollamaResp.Model,
		Content:      ollamaResp.Choices[0].Message.Content,
		InputTokens:  ollamaResp.Usage.PromptTokens,
		OutputTokens: ollamaResp.Usage.CompletionTokens,
	}
	if calls := ollamaResp.Choices[0].Message.ToolCalls; len(calls) > 0 {
		result.ToolCall = &ToolCall{Name: calls[0].Function.Name, Arguments: normalizeToolArguments(calls[0].Function.Arguments)}
	} else if call := parseOllamaContentToolCall(result.Content); call != nil {
		// Some Ollama model templates (including qwen2.5-coder) follow the
		// requested function contract but return the call in content instead of
		// OpenAI's tool_calls field. Normalize that well-defined envelope here so
		// the runtime still receives a typed action; arbitrary prose is rejected.
		result.ToolCall = call
		result.Content = ""
	}
	return result, nil
}

func parseOllamaContentToolCall(content string) *ToolCall {
	raw := strings.TrimSpace(content)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 && strings.HasPrefix(lines[0], "```") && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			raw = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	var envelope struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil ||
		strings.TrimSpace(envelope.Name) == "" || len(envelope.Arguments) == 0 ||
		!json.Valid(envelope.Arguments) {
		return nil
	}
	return &ToolCall{Name: strings.ToLower(strings.TrimSpace(envelope.Name)), Arguments: normalizeToolArguments(envelope.Arguments)}
}

func (p *Ollama) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
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

		var chunk ollamaStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- StreamEvent{Error: fmt.Errorf("parse stream event: %w", err)}
			return
		}

		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
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
