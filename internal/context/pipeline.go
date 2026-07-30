package context

import (
	"context"
	"strings"

	"github.com/saterdoe/oberth/internal/vault"
)

// Level represents the depth of context compilation.
type Level int

const (
	// LevelSimple ~200 tokens: only memory-index
	LevelSimple Level = iota
	// LevelMedium ~500 tokens: memory-index + top note
	LevelMedium
	// LevelComplex ~2000 tokens: full relevant context
	LevelComplex
)

// CompileResult contains the compiled context and its sources.
type CompileResult struct {
	SchemaVersion string             `json:"schema_version"`
	Context       string             `json:"context"`
	Sources       []string           `json:"sources"`
	Level         Level              `json:"level"`
	Tokens        int                `json:"tokens_estimate"`
	Metrics       CompileMetrics     `json:"metrics"`
	Manifest      []SourceSelection  `json:"manifest"`
	Exclusions    []ContextExclusion `json:"exclusions,omitempty"`
}

type ContextExclusion struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// taskTypeDirs maps task types to the vault directories from which notes should
// be loaded when compiling context for that task type.
var taskTypeDirs = map[string][]string{
	"architecture": {"architecture", "decisions"},
	"bug_fix":      {"bugs", "patterns"},
	"review":       {"patterns", "decisions"},
	"docs":         {"architecture", "sessions"},
}

// Pipeline compiles context for an LLM request.
type Pipeline struct {
	vault    *vault.Vault
	searcher *Searcher
}

// NewPipeline creates a new Pipeline.
func NewPipeline(v *vault.Vault, s *Searcher) *Pipeline {
	return &Pipeline{vault: v, searcher: s}
}

// Compile builds context based on query complexity and task type.
// query: the user's request text
// taskType: type of task (architecture, bug, review, docs, etc.)
// Returns compiled context + sources used.
func (p *Pipeline) Compile(ctx context.Context, query string, taskType string) (*CompileResult, error) {
	level := determineLevel(query, taskType)

	miContent := p.readMemoryIndex(ctx)

	parts := []string{}
	sources := []string{}

	if miContent != "" {
		parts = append(parts, miContent)
		sources = append(sources, "memory-index")
	}

	if level > LevelSimple {
		dirs := taskTypeDirs[taskType]
		limit := 1
		if level == LevelComplex {
			limit = 5
		}

		results, err := p.searcher.Search(ctx, query, limit*3, "")
		if err == nil {
			for _, r := range results {
				if len(sources)-1 >= limit {
					break
				}
				if dirs == nil || hasPathPrefixInList(r.Note.Path, dirs) {
					parts = append(parts, r.Note.Content)
					sources = append(sources, r.Note.Path)
				}
			}
		}
	}

	contextStr := strings.TrimSpace(strings.Join(parts, "\n\n---\n\n"))
	tokens := estimateTokens(contextStr)

	return &CompileResult{
		SchemaVersion: "1",
		Context:       contextStr,
		Sources:       sources,
		Level:         level,
		Tokens:        tokens,
	}, nil
}

// determineLevel returns the compilation level based on query length and task type.
func determineLevel(query string, taskType string) Level {
	if taskType == "architecture" || taskType == "review" || len(query) > 300 {
		return LevelComplex
	}
	if len(query) >= 100 {
		return LevelMedium
	}
	return LevelSimple
}

func (p *Pipeline) readMemoryIndex(ctx context.Context) string {
	if p == nil || p.vault == nil {
		return ""
	}
	note, err := p.vault.ReadNote("memory-index")
	if err != nil {
		return ""
	}
	return note.Content
}

func hasPathPrefixInList(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if s == p || strings.HasPrefix(s, p+"/") {
			return true
		}
	}
	return false
}
