package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Tool defines an MCP tool with its name, description, input JSON schema,
// and handler function.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Handler     func(ctx context.Context, args json.RawMessage) (any, error)
}

// jsonrpcRequest represents a JSON-RPC 2.0 request.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse represents a JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError represents a JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Server handles MCP JSON-RPC messages for tool invocation.
type Server struct {
	tools map[string]Tool
}

// NewServer creates a new MCP server with no registered tools.
func NewServer() *Server {
	return &Server{tools: make(map[string]Tool)}
}

// RegisterTool registers a tool with the server so it can be called
// via JSON-RPC messages.
func (s *Server) RegisterTool(t Tool) {
	s.tools[t.Name] = t
}

// Tools returns the registered tools without exposing handler functions.
func (s *Server) Tools() []Tool {
	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tool.Handler = nil
		tools = append(tools, tool)
	}
	return tools
}

// HandleMessage reads a single JSON-RPC message from r, processes it,
// and writes the JSON-RPC response to w.
func (s *Server) HandleMessage(ctx context.Context, r io.Reader, w io.Writer) error {
	var req jsonrpcRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		writeError(w, nil, -32700, "Parse error")
		return fmt.Errorf("parse error: %w", err)
	}

	if req.JSONRPC != "2.0" {
		writeError(w, req.ID, -32600, "Invalid Request: jsonrpc must be 2.0")
		return errors.New("invalid jsonrpc version")
	}

	if req.ID == nil {
		writeError(w, nil, -32600, "Invalid Request: id is required")
		return errors.New("missing id")
	}

	switch req.Method {
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if req.Params != nil {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeError(w, req.ID, -32602, "Invalid params")
				return fmt.Errorf("invalid params: %w", err)
			}
		} else {
			writeError(w, req.ID, -32602, "Invalid params")
			return errors.New("missing params")
		}

		tool, ok := s.tools[params.Name]
		if !ok {
			writeError(w, req.ID, -32601, fmt.Sprintf("method not found: %s", params.Name))
			return fmt.Errorf("tool not found: %s", params.Name)
		}

		result, err := tool.Handler(ctx, params.Arguments)
		if err != nil {
			writeError(w, req.ID, -32000, err.Error())
			return err
		}

		return writeResult(w, req.ID, wrapResult(result))

	default:
		writeError(w, req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
		return fmt.Errorf("method not found: %s", req.Method)
	}
}

// ServeStdio listens on stdin for JSON-RPC messages (one per line) and
// writes responses to stdout.
func (s *Server) ServeStdio(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var buf bytes.Buffer
		_ = s.HandleMessage(ctx, bytes.NewReader(line), &buf)
		fmt.Fprintln(os.Stdout, buf.String())
	}
	return scanner.Err()
}

// wrapResult wraps a handler result in the MCP content format.
// If the result is already a map with a "content" key it is passed through.
func wrapResult(result any) any {
	if m, ok := result.(map[string]any); ok {
		if _, has := m["content"]; has {
			return m
		}
	}

	var text string
	switch v := result.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		b, err := json.Marshal(result)
		if err != nil {
			text = fmt.Sprintf("%v", result)
		} else {
			text = string(b)
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

// writeResult writes a JSON-RPC success response to w.
func writeResult(w io.Writer, id json.RawMessage, result any) error {
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return json.NewEncoder(w).Encode(resp)
}

// writeError writes a JSON-RPC error response to w.
func writeError(w io.Writer, id json.RawMessage, code int, message string) error {
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonrpcError{
			Code:    code,
			Message: message,
		},
	}
	return json.NewEncoder(w).Encode(resp)
}
