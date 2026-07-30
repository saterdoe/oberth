package structuredoutput

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/saterdoe/oberth/pkg/llm"
)

type NativeEngine struct {
	provider llm.Provider
	logger   Logger
	model    string
}

func NewNativeEngine(provider llm.Provider, logger Logger, model string) *NativeEngine {
	return &NativeEngine{provider: provider, logger: logger, model: model}
}

func (e *NativeEngine) Name() string { return "native_json" }

func (e *NativeEngine) ClassifyTask(ctx context.Context, input ClassifyTaskInput) (*ClassifyTaskOutput, error) {
	schema := `{
  "task_type": "implementation | review | docs | architecture | debug | research",
  "confidence": 0.0-1.0,
  "risk_level": "low | medium | high | critical",
  "requires_git_changes": true/false,
  "requires_human_approval": true/false,
  "suggested_execution_mode": "direct | plan_then_diff | review_required"
}`
	out := &ClassifyTaskOutput{}
	err := e.call(ctx, "classify_task", fmt.Sprintf(
		`Classify the following task and return JSON matching this schema:
%s

Task description: %s
Branch: %s`,
		schema, input.TaskDescription, input.CurrentBranch), out)
	return out, err
}

func (e *NativeEngine) SelectContext(ctx context.Context, input SelectContextInput) (*SelectContextOutput, error) {
	schema := `{
  "selected_sources": ["..."],
  "rejected_sources": ["..."],
  "reason": "...",
  "confidence": 0.0-1.0,
  "estimated_tokens": 0
}`
	out := &SelectContextOutput{}
	notes := strings.Join(input.CandidateNotes, "\n")
	err := e.call(ctx, "select_context", fmt.Sprintf(
		`Select the most relevant context sources for the given task. Return JSON matching:
%s

Task: %s
Description: %s
Candidate notes:
%s`,
		schema, input.Task, input.TaskDescription, notes), out)
	return out, err
}

func (e *NativeEngine) ReviewPatch(ctx context.Context, input ReviewPatchInput) (*ReviewPatchOutput, error) {
	schema := `{
  "approved": true/false,
  "severity": "low | medium | high | critical",
  "blocking_issues": ["..."],
  "comments_by_file": [{"file_path": "...", "line": 0, "comment": "...", "severity": "..."}],
  "required_changes": ["..."],
  "should_retry_generation": true/false
}`
	out := &ReviewPatchOutput{}
	err := e.call(ctx, "review_patch", fmt.Sprintf(
		`Review the following patch and return JSON matching:
%s

Task: %s
Description: %s
Patch:
%s`,
		schema, input.Task, input.TaskDescription, input.Patch), out)
	return out, err
}

func (e *NativeEngine) DetectADRCandidate(ctx context.Context, input DetectADRCandidateInput) (*DetectADRCandidateOutput, error) {
	schema := `{
  "should_create_adr": true/false,
  "title": "...",
  "decision": "...",
  "context": "...",
  "alternatives": "...",
  "consequences": "...",
  "confidence": 0.0-1.0
}`
	out := &DetectADRCandidateOutput{}
	decisions := strings.Join(input.UserDecisions, "\n")
	err := e.call(ctx, "detect_adr_candidate", fmt.Sprintf(
		`Detect if this session produced an architecture decision. Return JSON matching:
%s

Session summary: %s
Changes: %s
User decisions: %s`,
		schema, input.SessionSummary, input.GeneratedChanges, decisions), out)
	return out, err
}

func (e *NativeEngine) ScoreRisk(ctx context.Context, input ScoreRiskInput) (*ScoreRiskOutput, error) {
	schema := `{
  "risk_level": "low | medium | high | critical",
  "reasons": ["..."],
  "requires_approval": true/false,
  "cloud_allowed": true/false,
  "required_checks": ["..."]
}`
	out := &ScoreRiskOutput{}
	files := strings.Join(input.FilesToModify, "\n")
	err := e.call(ctx, "score_risk", fmt.Sprintf(
		`Score the risk level of the following task. Return JSON matching:
%s

Task: %s
Description: %s
Files to modify: %s
Model: %s`,
		schema, input.Task, input.TaskDescription, files, input.SelectedModel), out)
	return out, err
}

func (e *NativeEngine) call(ctx context.Context, function string, prompt string, out any) error {
	start := time.Now()

	sysMsg := `You are a structured output generator. Always respond with valid JSON only, no markdown, no explanation.`
	resp, err := e.provider.Chat(ctx, llm.ChatRequest{
		Model: e.model,
		Messages: []llm.Message{
			{Role: "system", Content: sysMsg},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
	})

	latency := time.Since(start).Milliseconds()
	tokensIn := 0
	tokensOut := 0
	if resp != nil {
		tokensIn = resp.InputTokens
		tokensOut = resp.OutputTokens
	}

	if err != nil {
		e.logger.LogTrace(function, prompt, nil, err, latency, tokensIn, tokensOut)
		return fmt.Errorf("%s: %w", function, err)
	}

	content := resp.Content
	content = stripCodeFences(content)
	content = strings.ReplaceAll(content, "\\`", "`")
	content = escapeNewlinesInJSON(content)

	if err := json.Unmarshal([]byte(content), out); err != nil {
		e.logger.LogTrace(function, prompt, content, err, latency, tokensIn, tokensOut)
		return fmt.Errorf("%s parse: %w", function, err)
	}

	e.logger.LogTrace(function, prompt, out, nil, latency, tokensIn, tokensOut)
	return nil
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
			lines = lines[:len(lines)-1]
		}
		s = strings.Join(lines, "\n")
	}
	return strings.TrimSpace(s)
}

// escapeNewlinesInJSON replaces unescaped newlines (\n and \r) inside JSON
// string values with their escaped form (\\n), fixing a common LLM mistake.
func escapeNewlinesInJSON(raw string) string {
	var b strings.Builder
	b.Grow(len(raw) + 64)
	inStr := false
	esc := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if esc {
			b.WriteByte(c)
			esc = false
			continue
		}
		switch c {
		case '\\':
			esc = true
			b.WriteByte(c)
		case '"':
			inStr = !inStr
			b.WriteByte(c)
		case '\n', '\r':
			if inStr {
				b.WriteString("\\n")
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
