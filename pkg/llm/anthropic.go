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

type Anthropic struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewAnthropic(apiKey string) *Anthropic {
	if apiKey == "" {
		apiKey = os.Getenv("OBERTH_ANTHROPIC_API_KEY")
	}
	return &Anthropic{
		baseURL:    "https://api.anthropic.com/v1",
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *Anthropic) Name() string {
	return "anthropic"
}

func (p *Anthropic) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
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

func (p *Anthropic) ChatStream(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
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

type anthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Tools       []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	} `json:"tools,omitempty"`
}

type anthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []anthropicContent `json:"content"`
	Model      string             `json:"model"`
	StopReason string             `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type       string `json:"type"`
		Text       string `json:"text"`
		StopReason string `json:"stop_reason"`
	} `json:"delta,omitempty"`
	ContentBlock *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content_block,omitempty"`
	Message *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Role  string `json:"role"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	} `json:"message,omitempty"`
}

func (p *Anthropic) buildRequest(ctx context.Context, req ChatRequest, stream bool) (*http.Request, error) {
	systemPrompt := ""
	var msgs []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			if systemPrompt != "" {
				systemPrompt += "\n" + m.Content
			} else {
				systemPrompt = m.Content
			}
			continue
		}
		msgs = append(msgs, anthropicMessage{
			Role:    m.Role,
			Content: []anthropicContent{{Type: "text", Text: m.Content}},
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := anthropicRequest{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		Messages:    msgs,
		Temperature: req.Temperature,
		Stream:      stream,
	}
	if systemPrompt != "" {
		body.System = systemPrompt
	}
	for _, tool := range req.Tools {
		body.Tools = append(body.Tools, struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"input_schema"`
		}{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	return httpReq, nil
}

func (p *Anthropic) parseResponse(resp *http.Response) (*ChatResponse, error) {
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidAPIKey
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return nil, errors.New("no content in response")
	}

	content := ""
	var toolCall *ToolCall
	for _, c := range anthropicResp.Content {
		if c.Type == "text" {
			content += c.Text
		} else if c.Type == "tool_use" && toolCall == nil {
			toolCall = &ToolCall{Name: c.Name, Arguments: c.Input}
		}
	}

	return &ChatResponse{
		Model:        anthropicResp.Model,
		Content:      content,
		InputTokens:  anthropicResp.Usage.InputTokens,
		OutputTokens: anthropicResp.Usage.OutputTokens,
		ToolCall:     toolCall,
	}, nil
}

func (p *Anthropic) readStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			ch <- StreamEvent{Error: fmt.Errorf("parse stream event: %w", err)}
			return
		}

		switch event.Type {
		case "content_block_delta":
			if event.Delta != nil && event.Delta.Text != "" {
				select {
				case ch <- StreamEvent{Content: event.Delta.Text}:
				case <-ctx.Done():
					ch <- StreamEvent{Error: ErrTimeout}
					return
				}
			}
		case "message_stop":
			ch <- StreamEvent{Done: true}
			return
		case "error":
			ch <- StreamEvent{Error: fmt.Errorf("anthropic stream error: %s", data)}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case ch <- StreamEvent{Error: fmt.Errorf("stream read error: %w", err)}:
		case <-ctx.Done():
		}
	}
}
