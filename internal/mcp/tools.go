package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saterdoe/oberth/internal/vault"
)

// VaultToolsOptions configures optional backends for vault tools.
type VaultToolsOptions struct {
	// SearchFn performs a semantic search. If nil, falls back to keyword match.
	SearchFn func(ctx context.Context, query string, limit int, taskType string) ([]map[string]any, error)
	// CompileContextFn compiles context for a task and query. If nil, returns memory-index.
	CompileContextFn func(ctx context.Context, query string, taskType string) (any, error)
}

// NewVaultTools creates a set of MCP tools backed by a vault.Vault.
func NewVaultTools(v *vault.Vault, opts *VaultToolsOptions) []Tool {
	if opts == nil {
		opts = &VaultToolsOptions{}
	}

	return []Tool{
		{
			Name:        "read-note",
			Description: "Read a note from the vault by its path",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
				var params struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}
				note, err := v.ReadNote(params.Path)
				if err != nil {
					return nil, err
				}
				return note, nil
			},
		},
		{
			Name:        "search-vault",
			Description: "Search the vault semantically",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"},"task_type":{"type":"string"}},"required":["query"]}`),
			Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
				var params struct {
					Query    string `json:"query"`
					Limit    int    `json:"limit"`
					TaskType string `json:"task_type"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}
				if params.Limit <= 0 {
					params.Limit = 5
				}

				if opts.SearchFn != nil {
					results, err := opts.SearchFn(ctx, params.Query, params.Limit, params.TaskType)
					if err != nil {
						return nil, fmt.Errorf("search failed: %w", err)
					}
					return map[string]any{"results": results}, nil
				}

				notes, err := v.ListNotes("")
				if err != nil {
					return nil, err
				}
				queryLower := strings.ToLower(params.Query)
				var results []map[string]any
				for _, note := range notes {
					if params.TaskType != "" {
						nt, ok := note.Metadata["type"].(string)
						if !ok || nt != params.TaskType {
							continue
						}
					}
					if note.Content != "" && strings.Contains(strings.ToLower(note.Content), queryLower) {
						results = append(results, map[string]any{
							"note":  note,
							"score": 0.5,
						})
					}
					if len(results) >= params.Limit {
						break
					}
				}
				return map[string]any{"results": results}, nil
			},
		},
		{
			Name:        "get-context",
			Description: "Compile context for a task type",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"task_type":{"type":"string"},"query":{"type":"string"}},"required":["task_type"]}`),
			Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
				var params struct {
					TaskType string `json:"task_type"`
					Query    string `json:"query"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return nil, fmt.Errorf("invalid arguments: %w", err)
				}

				if opts.CompileContextFn != nil {
					query := params.Query
					if query == "" {
						query = params.TaskType
					}
					compiled, err := opts.CompileContextFn(ctx, query, params.TaskType)
					if err != nil {
						return nil, fmt.Errorf("context compilation failed: %w", err)
					}
					return compiled, nil
				}

				note, err := v.ReadNote("memory-index")
				if err != nil {
					return map[string]any{"context": "", "sources": []string{}, "level": 0, "tokens_estimate": 0}, nil
				}
				return map[string]any{
					"context":         note.Content,
					"sources":         []string{"memory-index"},
					"level":           0,
					"tokens_estimate": len(strings.Fields(note.Content)),
				}, nil
			},
		},
		{
			Name:        "get-memory-index",
			Description: "Return the memory-index content",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
				note, err := v.ReadNote("memory-index")
				if err != nil {
					return map[string]any{"content": ""}, nil
				}
				return note, nil
			},
		},
	}
}
