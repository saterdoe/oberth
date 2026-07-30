package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIProvider_Name(t *testing.T) {
	p := NewOpenAI("https://api.openai.com/v1", "test-key")
	assert.Equal(t, "openai", p.Name())
}

func TestOllamaProvider_Name(t *testing.T) {
	p := NewOllama("http://localhost:11434/v1")
	assert.Equal(t, "ollama", p.Name())
}

func TestOllamaNormalizesContentToolEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "qwen2.5-coder:7b",
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant", "content": `{"name":"Read","arguments":{"path":"README.md"}}`,
			}}},
		})
	}))
	defer server.Close()

	response, err := NewOllama(server.URL).Chat(context.Background(), ChatRequest{
		Model: "qwen2.5-coder:7b",
		Tools: []ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	require.NoError(t, err)
	require.NotNil(t, response.ToolCall)
	assert.Equal(t, "read", response.ToolCall.Name)
	assert.JSONEq(t, `{"path":"README.md"}`, string(response.ToolCall.Arguments))
	assert.Empty(t, response.Content)
}

func TestOllamaDoesNotTreatArbitraryJSONAsToolCall(t *testing.T) {
	assert.Nil(t, parseOllamaContentToolCall(`{"answer":"README.md"}`))
	assert.Nil(t, parseOllamaContentToolCall(`I would read README.md`))
}

func TestOpenAIChat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4", reqBody["model"])
		assert.NotEqual(t, true, reqBody["stream"])

		resp := map[string]interface{}{
			"id":     "chatcmpl-123",
			"object": "chat.completion",
			"model":  "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 20,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenAI(server.URL+"/v1", "test-key")
	resp, err := p.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "gpt-4", resp.Model)
	assert.Equal(t, "Hello!", resp.Content)
	assert.Equal(t, 10, resp.InputTokens)
	assert.Equal(t, 20, resp.OutputTokens)
}

func TestOpenAIChatNativeToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body["tools"], 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-test",
			"choices": []any{map[string]any{"message": map[string]any{
				"role": "assistant", "content": "",
				"tool_calls": []any{map[string]any{"function": map[string]any{
					"name": "read", "arguments": `{"path":"main.go"}`,
				}}},
			}}},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 3},
		})
	}))
	defer server.Close()
	provider := NewOpenAI(server.URL, "key")
	response, err := provider.Chat(context.Background(), ChatRequest{
		Model: "gpt-test", Messages: []Message{{Role: "user", Content: "inspect"}},
		Tools: []ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	require.NoError(t, err)
	require.NotNil(t, response.ToolCall)
	assert.Equal(t, "read", response.ToolCall.Name)
	assert.JSONEq(t, `{"path":"main.go"}`, string(response.ToolCall.Arguments))
}

func TestOllamaChat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"))

		resp := map[string]interface{}{
			"id":     "chatcmpl-123",
			"object": "chat.completion",
			"model":  "llama3",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello from Ollama!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     5,
				"completion_tokens": 15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllama(server.URL + "/v1")
	resp, err := p.Chat(context.Background(), ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "llama3", resp.Model)
	assert.Equal(t, "Hello from Ollama!", resp.Content)
	assert.Equal(t, 5, resp.InputTokens)
	assert.Equal(t, 15, resp.OutputTokens)
}

func TestOpenAIChat_InvalidAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API key"})
	}))
	defer server.Close()

	p := NewOpenAI(server.URL+"/v1", "bad-key")
	_, err := p.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAPIKey)
}

func TestOpenAIChat_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Internal error"})
	}))
	defer server.Close()

	p := NewOpenAI(server.URL+"/v1", "test-key")
	_, err := p.Chat(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	assert.Error(t, err)
}

func TestOllamaChat_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Internal error"})
	}))
	defer server.Close()

	p := NewOllama(server.URL + "/v1")
	_, err := p.Chat(context.Background(), ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	assert.Error(t, err)
}

func TestOpenAIChat_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewOpenAI(server.URL+"/v1", "test-key")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := p.Chat(ctx, ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTimeout)
}

func TestOllamaChat_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewOllama(server.URL + "/v1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := p.Chat(ctx, ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrTimeout)
}

func TestOpenAIChatStream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		assert.Equal(t, true, reqBody["stream"])

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		chunks := []string{"Hello", " world", "!"}
		for i, chunk := range chunks {
			event := map[string]interface{}{
				"id":     "chatcmpl-123",
				"object": "chat.completion.chunk",
				"model":  "gpt-4",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]string{
							"content": chunk,
						},
						"finish_reason": nil,
					},
				},
			}
			if i == len(chunks)-1 {
				s := "stop"
				event["choices"].([]map[string]interface{})[0]["finish_reason"] = &s
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewOpenAI(server.URL+"/v1", "test-key")
	events, err := p.ChatStream(context.Background(), ChatRequest{
		Model:    "gpt-4",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	require.NoError(t, err)

	var content strings.Builder
	var done bool
	for evt := range events {
		if evt.Error != nil {
			t.Fatal(evt.Error)
		}
		content.WriteString(evt.Content)
		if evt.Done {
			done = true
		}
	}
	assert.True(t, done)
	assert.Equal(t, "Hello world!", content.String())
}

func TestOllamaChatStream_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"))

		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		chunks := []string{"Hi", " there"}
		for _, chunk := range chunks {
			event := map[string]interface{}{
				"id":     "chatcmpl-123",
				"object": "chat.completion.chunk",
				"model":  "llama3",
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]string{
							"content": chunk,
						},
						"finish_reason": nil,
					},
				},
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		s := "stop"
		event := map[string]interface{}{
			"id":     "chatcmpl-123",
			"object": "chat.completion.chunk",
			"model":  "llama3",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]string{},
					"finish_reason": &s,
				},
			},
		}
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewOllama(server.URL + "/v1")
	events, err := p.ChatStream(context.Background(), ChatRequest{
		Model:    "llama3",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	require.NoError(t, err)

	var content strings.Builder
	var done bool
	for evt := range events {
		if evt.Error != nil {
			t.Fatal(evt.Error)
		}
		content.WriteString(evt.Content)
		if evt.Done {
			done = true
		}
	}
	assert.True(t, done)
	assert.Equal(t, "Hi there", content.String())
}
