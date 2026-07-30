package context_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/vector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipeline_CompileSimple(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("memory-index", "# Memory Index\n\n## Summary\n\n## Notes\n- [note1](note1)", map[string]any{})
	require.NoError(t, err)
	_, err = v2.CreateNote("other", "unrelated content", map[string]any{"type": "task"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	result, err := p.Compile(context.Background(), "hello", "")
	require.NoError(t, err)
	assert.Equal(t, pkgctx.LevelSimple, result.Level)
	assert.Equal(t, []string{"memory-index"}, result.Sources)
	assert.Contains(t, result.Context, "Memory Index")
	assert.NotContains(t, result.Context, "unrelated content")
}

func TestPipeline_CompileMedium(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("memory-index", "# Memory Index\n\n## Summary\n\n## Notes", map[string]any{})
	require.NoError(t, err)
	_, err = v2.CreateNote("architecture/api-design", "REST API design principles and best practices", map[string]any{"type": "architecture"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	err = mvs.Upsert(context.Background(), []vector.Point{
		{
			ID:     "architecture/api-design:0",
			Vector: []float32{0.1, 0.2, 0.3},
			Payload: map[string]any{
				"note_path": "architecture/api-design",
				"chunk_idx": 0,
			},
		},
	})
	require.NoError(t, err)

	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	query := strings.Repeat("x", 150)
	result, err := p.Compile(context.Background(), query, "")
	require.NoError(t, err)
	assert.Equal(t, pkgctx.LevelMedium, result.Level)
	require.Len(t, result.Sources, 2)
	assert.Equal(t, "memory-index", result.Sources[0])
	assert.Contains(t, result.Context, "Memory Index")
	assert.Contains(t, result.Context, "REST API")
}

func TestPipeline_CompileComplex(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("memory-index", "# Memory Index\n\n## Summary\n\n## Notes", map[string]any{})
	require.NoError(t, err)
	_, err = v2.CreateNote("architecture/api-design", "REST API design principles", map[string]any{"type": "architecture"})
	require.NoError(t, err)
	_, err = v2.CreateNote("decisions/tech-stack", "Go and Postgres tech stack decision", map[string]any{"type": "decision"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	for _, pt := range []vector.Point{
		{ID: "architecture/api-design:0", Vector: []float32{0.1, 0.2, 0.3}, Payload: map[string]any{"note_path": "architecture/api-design", "chunk_idx": 0}},
		{ID: "decisions/tech-stack:0", Vector: []float32{0.4, 0.5, 0.6}, Payload: map[string]any{"note_path": "decisions/tech-stack", "chunk_idx": 0}},
	} {
		err = mvs.Upsert(context.Background(), []vector.Point{pt})
		require.NoError(t, err)
	}

	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	query := strings.Repeat("x", 350)
	result, err := p.Compile(context.Background(), query, "")
	require.NoError(t, err)
	assert.Equal(t, pkgctx.LevelComplex, result.Level)
	assert.GreaterOrEqual(t, len(result.Sources), 2)
	assert.Contains(t, result.Context, "Memory Index")
	assert.Contains(t, result.Context, "REST API")
}

func TestPipeline_EmptyVault(t *testing.T) {
	v, _ := setupTestVault(t)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	result, err := p.Compile(context.Background(), "hello", "")
	require.NoError(t, err)
	assert.Equal(t, pkgctx.LevelSimple, result.Level)
	assert.Empty(t, result.Sources)
	assert.Empty(t, result.Context)
}

func TestPipeline_TaskTypeFiltering(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("memory-index", "# Memory Index\n\n## Summary\n\n## Notes", map[string]any{})
	require.NoError(t, err)
	_, err = v2.CreateNote("architecture/api-design", "REST API design", map[string]any{"type": "architecture"})
	require.NoError(t, err)
	_, err = v2.CreateNote("bugs/login-error", "login bug fix", map[string]any{"type": "bug"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	for _, pt := range []vector.Point{
		{ID: "architecture/api-design:0", Vector: []float32{0.1, 0.2, 0.3}, Payload: map[string]any{"note_path": "architecture/api-design", "chunk_idx": 0}},
		{ID: "bugs/login-error:0", Vector: []float32{0.4, 0.5, 0.6}, Payload: map[string]any{"note_path": "bugs/login-error", "chunk_idx": 0}},
	} {
		err = mvs.Upsert(context.Background(), []vector.Point{pt})
		require.NoError(t, err)
	}

	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	query := strings.Repeat("x", 150)
	result, err := p.Compile(context.Background(), query, "bug_fix")
	require.NoError(t, err)
	assert.Equal(t, pkgctx.LevelMedium, result.Level)
	assert.Contains(t, result.Context, "Memory Index")
	assert.Contains(t, result.Context, "login bug fix")
	assert.NotContains(t, result.Context, "REST API")
	assert.Contains(t, result.Sources, "bugs/login-error")
	assert.NotContains(t, result.Sources, "architecture/api-design")
}

func TestPipeline_TokenEstimation(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("memory-index", "# Memory Index\n\n## Summary\n\n## Notes", map[string]any{})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	result, err := p.Compile(context.Background(), "hello", "")
	require.NoError(t, err)
	assert.Equal(t, pkgctx.LevelSimple, result.Level)
	expectedTokens := len(result.Context) / 4
	if expectedTokens < 1 {
		expectedTokens = 1
	}
	assert.Equal(t, expectedTokens, result.Tokens)
}

func TestPipeline_ComplexViaTaskType(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("memory-index", "# Memory Index\n\n## Summary\n\n## Notes", map[string]any{})
	require.NoError(t, err)
	_, err = v2.CreateNote("patterns/observer", "observer pattern implementation", map[string]any{"type": "pattern"})
	require.NoError(t, err)
	_, err = v2.CreateNote("decisions/tech-stack", "Go tech stack decision", map[string]any{"type": "decision"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	for _, pt := range []vector.Point{
		{ID: "patterns/observer:0", Vector: []float32{0.1, 0.2, 0.3}, Payload: map[string]any{"note_path": "patterns/observer", "chunk_idx": 0}},
		{ID: "decisions/tech-stack:0", Vector: []float32{0.4, 0.5, 0.6}, Payload: map[string]any{"note_path": "decisions/tech-stack", "chunk_idx": 0}},
	} {
		err = mvs.Upsert(context.Background(), []vector.Point{pt})
		require.NoError(t, err)
	}

	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	result, err := p.Compile(context.Background(), "hello", "architecture")
	require.NoError(t, err)
	assert.Equal(t, pkgctx.LevelComplex, result.Level)
}

func TestPipeline_NoMemoryIndex(t *testing.T) {
	v, _ := setupTestVault(t)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	p := pkgctx.NewPipeline(v, s)

	result, err := p.Compile(context.Background(), "hello", "")
	require.NoError(t, err)
	assert.Empty(t, result.Sources)
	assert.Empty(t, result.Context)
}
