package structuredoutput

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/pkg/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── stripCodeFences ────────────────────────────────────────────

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no fences", `{"a": 1}`, `{"a": 1}`},
		{"with fences", "```\n{\"a\": 1}\n```", `{"a": 1}`},
		{"with fences and language", "```json\n{\"a\": 1}\n```", `{"a": 1}`},
		{"nested braces in fences", "```\n{\"a\": {\"b\": 2}}\n```", `{"a": {"b": 2}}`},
		{"empty string", "", ""},
		{"only fences", "```\ncontent\n```", "content"},
		{"no closing fence", "```\ncontent", "content"},
		{"already trimmed", `{"x":1}`, `{"x":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCodeFences(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ── NopLogger ──────────────────────────────────────────────────

func TestNopLogger(t *testing.T) {
	l := NopLogger{}
	l.LogTrace("test", "input", "output", nil, 100, 10, 20)
	l.LogTrace("err", nil, nil, assert.AnError, 0, 0, 0)
}

// ── FileLogger ─────────────────────────────────────────────────

func TestNewFileLogger_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewFileLogger(filepath.Join(dir, "traces"))
	require.NoError(t, err)
	require.NotNil(t, logger)
	assert.DirExists(t, filepath.Join(dir, "traces"))
}

func TestFileLogger_LogTrace_WritesFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewFileLogger(dir)
	require.NoError(t, err)

	logger.LogTrace("test_fn", "my_input", "my_output", nil, 42, 10, 20)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	require.NoError(t, err)

	var entry TraceEntry
	err = json.Unmarshal(data, &entry)
	require.NoError(t, err)

	assert.Equal(t, "test_fn", entry.Function)
	assert.Equal(t, "0.1", entry.SchemaVersion)
	assert.Equal(t, "my_input", entry.Input)
	assert.Equal(t, "my_output", entry.Output)
	assert.Equal(t, int64(42), entry.LatencyMs)
	assert.Equal(t, 10, entry.TokensIn)
	assert.Equal(t, 20, entry.TokensOut)
	assert.Empty(t, entry.Error)
	assert.NotEmpty(t, entry.Timestamp)
}

func TestFileLogger_LogTrace_WithError(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewFileLogger(dir)
	require.NoError(t, err)

	expectedErr := assert.AnError
	logger.LogTrace("err_fn", nil, nil, expectedErr, 0, 0, 0)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	require.NoError(t, err)

	var entry TraceEntry
	err = json.Unmarshal(data, &entry)
	require.NoError(t, err)
	assert.Equal(t, expectedErr.Error(), entry.Error)
}

func TestFileLogger_IncrementsCounter(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewFileLogger(dir)
	require.NoError(t, err)

	logger.LogTrace("a", nil, nil, nil, 0, 0, 0)
	logger.LogTrace("b", nil, nil, nil, 0, 0, 0)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

// ── NewManager ─────────────────────────────────────────────────

func TestNewManager_Disabled(t *testing.T) {
	cfg := config.StructuredOutputsConfig{Enabled: false}
	m, err := NewManager(cfg, nil)
	require.NoError(t, err)
	assert.False(t, m.Enabled)
	assert.NotNil(t, m.Logger)
}

func TestNewManager_Disabled_NoProvider(t *testing.T) {
	cfg := config.StructuredOutputsConfig{Enabled: false}
	m, err := NewManager(cfg, nil)
	require.NoError(t, err)
	assert.False(t, m.Enabled)
}

func TestNewManager_NativeEngine_WithProvider(t *testing.T) {
	cfg := config.StructuredOutputsConfig{
		Enabled: true,
		Engine:  "native_json",
	}
	provider := &mockProvider{}
	m, err := NewManager(cfg, provider)
	require.NoError(t, err)
	assert.True(t, m.Enabled)
	assert.Equal(t, "native_json", m.EngineName)
	assert.NotNil(t, m.Engine)
}

func TestNewManager_NativeEngine_Default(t *testing.T) {
	cfg := config.StructuredOutputsConfig{
		Enabled: true,
		Engine:  "",
	}
	m, err := NewManager(cfg, nil)
	require.NoError(t, err)
	assert.True(t, m.Enabled)
	assert.Equal(t, "", m.EngineName)
	assert.NotNil(t, m.Engine)
}

func TestNewManager_BAMLEngine(t *testing.T) {
	cfg := config.StructuredOutputsConfig{
		Enabled: true,
		Engine:  "baml",
	}
	m, err := NewManager(cfg, nil)
	require.NoError(t, err)
	assert.True(t, m.Enabled)
	assert.Equal(t, "baml", m.EngineName)
	_, ok := m.Engine.(*BAMLEngine)
	assert.True(t, ok)
}

func TestNewManager_TracesEnabled_CreatesLogger(t *testing.T) {
	dir := t.TempDir()
	cfg := config.StructuredOutputsConfig{
		Enabled: false,
		Traces: config.TracesConfig{
			Enabled: true,
			Path:    dir,
		},
	}
	m, err := NewManager(cfg, nil)
	require.NoError(t, err)
	_, ok := m.Logger.(*FileLogger)
	assert.True(t, ok)
}

// ── NativeEngine ───────────────────────────────────────────────

type mockProvider struct {
	llm.Provider
	chatFunc func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.chatFunc != nil {
		return m.chatFunc(ctx, req)
	}
	return &llm.ChatResponse{Content: `{"task_type": "implementation", "confidence": 0.95, "risk_level": "low", "requires_git_changes": true, "requires_human_approval": false, "suggested_execution_mode": "plan_then_diff"}`, InputTokens: 50, OutputTokens: 100}, nil
}

func (m *mockProvider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, nil
}

func TestNativeEngine_Name(t *testing.T) {
	e := NewNativeEngine(&mockProvider{}, NopLogger{}, "gpt-4")
	assert.Equal(t, "native_json", e.Name())
}

func TestNativeEngine_ClassifyTask(t *testing.T) {
	e := NewNativeEngine(&mockProvider{}, NopLogger{}, "gpt-4")
	out, err := e.ClassifyTask(context.Background(), ClassifyTaskInput{
		TaskDescription: "Implement login feature",
		CurrentBranch:   "feature/login",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "implementation", out.TaskType)
	assert.Equal(t, 0.95, out.Confidence)
	assert.Equal(t, "low", out.RiskLevel)
	assert.True(t, out.RequiresGitChanges)
	assert.False(t, out.RequiresHumanApproval)
}

func TestNativeEngine_SelectContext(t *testing.T) {
	provider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Content:      `{"selected_sources": ["file1.go", "file2.go"], "rejected_sources": [], "reason": "relevant files", "confidence": 0.9, "estimated_tokens": 500}`,
				InputTokens:  30,
				OutputTokens: 60,
			}, nil
		},
	}
	e := NewNativeEngine(provider, NopLogger{}, "gpt-4")
	out, err := e.SelectContext(context.Background(), SelectContextInput{
		Task:            "fix bug",
		TaskDescription: "Fix null pointer in login",
		CandidateNotes:  []string{"file1.go", "file2.go"},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, []string{"file1.go", "file2.go"}, out.SelectedSources)
	assert.Equal(t, 500, out.EstimatedTokens)
}

func TestNativeEngine_ReviewPatch(t *testing.T) {
	provider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Content: `{"approved": false, "severity": "high", "blocking_issues": ["missing validation"], "comments_by_file": [{"file_path": "main.go", "line": 42, "comment": "add validation", "severity": "high"}], "required_changes": ["add input validation"], "should_retry_generation": true}`,
			}, nil
		},
	}
	e := NewNativeEngine(provider, NopLogger{}, "gpt-4")
	out, err := e.ReviewPatch(context.Background(), ReviewPatchInput{
		Task:  "fix validation",
		Patch: "diff --git a/main.go b/main.go\n+func validate() {}",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.False(t, out.Approved)
	assert.Equal(t, "high", out.Severity)
	assert.Len(t, out.BlockingIssues, 1)
	assert.Len(t, out.CommentsByFile, 1)
	assert.Equal(t, "main.go", out.CommentsByFile[0].FilePath)
	assert.True(t, out.ShouldRetryGeneration)
}

func TestNativeEngine_DetectADRCandidate(t *testing.T) {
	provider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Content: `{"should_create_adr": true, "title": "Use PostgreSQL", "decision": "Use PostgreSQL", "context": "Need relational DB", "alternatives": "MySQL", "consequences": "Better reliability", "confidence": 0.85}`,
			}, nil
		},
	}
	e := NewNativeEngine(provider, NopLogger{}, "gpt-4")
	out, err := e.DetectADRCandidate(context.Background(), DetectADRCandidateInput{
		SessionSummary:   "Discussed database options",
		GeneratedChanges: "Added PostgreSQL schema",
		UserDecisions:    []string{"Use PostgreSQL"},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.True(t, out.ShouldCreateADR)
	assert.Equal(t, "Use PostgreSQL", out.Title)
	assert.Equal(t, 0.85, out.Confidence)
}

func TestNativeEngine_ScoreRisk(t *testing.T) {
	provider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Content: `{"risk_level": "high", "reasons": ["modifies auth", "touches core"], "requires_approval": true, "cloud_allowed": false, "required_checks": ["security review", "unit tests"]}`,
			}, nil
		},
	}
	e := NewNativeEngine(provider, NopLogger{}, "gpt-4")
	out, err := e.ScoreRisk(context.Background(), ScoreRiskInput{
		Task:            "update auth",
		TaskDescription: "Update authentication module",
		FilesToModify:   []string{"auth.go", "auth_test.go"},
		SelectedModel:   "gpt-4",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "high", out.RiskLevel)
	assert.True(t, out.RequiresApproval)
	assert.False(t, out.CloudAllowed)
	assert.Len(t, out.RequiredChecks, 2)
}

func TestNativeEngine_Call_WithCodeFences(t *testing.T) {
	provider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Content: "```json\n{\"task_type\": \"debug\"}\n```",
			}, nil
		},
	}
	e := NewNativeEngine(provider, NopLogger{}, "gpt-4")
	out, err := e.ClassifyTask(context.Background(), ClassifyTaskInput{
		TaskDescription: "fix bug",
	})
	require.NoError(t, err)
	assert.Equal(t, "debug", out.TaskType)
}

func TestNativeEngine_Call_LLMError(t *testing.T) {
	provider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, assert.AnError
		},
	}
	e := NewNativeEngine(provider, NopLogger{}, "gpt-4")
	_, err := e.ClassifyTask(context.Background(), ClassifyTaskInput{
		TaskDescription: "test",
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestNativeEngine_Call_InvalidJSON(t *testing.T) {
	provider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{Content: "not json"}, nil
		},
	}
	e := NewNativeEngine(provider, NopLogger{}, "gpt-4")
	_, err := e.ClassifyTask(context.Background(), ClassifyTaskInput{
		TaskDescription: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

// ── BAMLEngine ─────────────────────────────────────────────────

func TestNewBAMLEngine(t *testing.T) {
	e := NewBAMLEngine(NopLogger{})
	assert.NotNil(t, e)
	assert.Equal(t, "baml", e.Name())
}

// ── fallbackClassifyTask ───────────────────────────────────────

func TestFallbackClassifyTask_NilProvider(t *testing.T) {
	out, err := fallbackClassifyTask(context.Background(), nil, ClassifyTaskInput{
		TaskDescription: "my task",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "my task", out.TaskType)
	assert.Equal(t, 1.0, out.Confidence)
	assert.Equal(t, "low", out.RiskLevel)
	assert.Equal(t, "direct", out.SuggestedExecutionMode)
}

func TestFallbackClassifyTask_WithProvider(t *testing.T) {
	out, err := fallbackClassifyTask(context.Background(), &mockProvider{}, ClassifyTaskInput{
		TaskDescription: "Implement login feature",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "implementation", out.TaskType)
}

// ── Manager.ClassifyTaskWithFallback ───────────────────────────

func TestClassifyTaskWithFallback_Disabled(t *testing.T) {
	cfg := config.StructuredOutputsConfig{Enabled: false}
	m, err := NewManager(cfg, nil)
	require.NoError(t, err)

	out, err := m.ClassifyTaskWithFallback(context.Background(), nil, ClassifyTaskInput{
		TaskDescription: "my task",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "my task", out.TaskType)
}

func TestClassifyTaskWithFallback_EngineError_FallbackSucceeds(t *testing.T) {
	badProvider := &mockProvider{
		chatFunc: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, assert.AnError
		},
	}
	cfg := config.StructuredOutputsConfig{Enabled: true, Engine: "native_json"}
	m, err := NewManager(cfg, badProvider)
	require.NoError(t, err)
	assert.True(t, m.Enabled)

	goodFallback := &mockProvider{}
	out, err := m.ClassifyTaskWithFallback(context.Background(), goodFallback, ClassifyTaskInput{
		TaskDescription: "Implement feature",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "implementation", out.TaskType)
	assert.Equal(t, 0.95, out.Confidence)
}

func TestClassifyTaskWithFallback_EngineSuccess(t *testing.T) {
	cfg := config.StructuredOutputsConfig{Enabled: true, Engine: "native_json"}
	m, err := NewManager(cfg, &mockProvider{})
	require.NoError(t, err)

	out, err := m.ClassifyTaskWithFallback(context.Background(), &mockProvider{}, ClassifyTaskInput{
		TaskDescription: "Implement feature",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "implementation", out.TaskType)
}

// ── TraceEntry JSON serialization ──────────────────────────────

func TestTraceEntry_JSONRoundTrip(t *testing.T) {
	entry := TraceEntry{
		Function:      "test",
		SchemaVersion: "0.1",
		Timestamp:     "2024-01-01T00:00:00Z",
		Input:         map[string]string{"key": "val"},
		Output:        "result",
		Error:         "",
		LatencyMs:     100,
		TokensIn:      10,
		TokensOut:     20,
		Engine:        "native_json",
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var decoded TraceEntry
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, entry.Function, decoded.Function)
	assert.Equal(t, entry.LatencyMs, decoded.LatencyMs)
	assert.Equal(t, entry.Error, decoded.Error)
}
