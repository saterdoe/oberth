package context_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/vector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockVectorStore struct {
	mu     sync.Mutex
	points []vector.Point
}

func TestPrivateLocalSemanticSearchWorksWithoutExternalServices(t *testing.T) {
	v, _ := setupTestVault(t)
	_, err := v.CreateNote("decisions/authentication", "OAuth authorization and secure login token rotation", map[string]any{"type": "decision"})
	require.NoError(t, err)
	_, err = v.CreateNote("patterns/rendering", "CSS layout and responsive interface components", map[string]any{"type": "pattern"})
	require.NoError(t, err)

	searcher := pkgctx.NewSearcher(v, nil, "")
	results, metrics, err := searcher.SearchWithMetrics(context.Background(), "authentication token login", 3, "")
	require.NoError(t, err)
	require.NotEmpty(t, results)
	assert.Equal(t, "decisions/authentication", results[0].Note.Path)
	assert.True(t, metrics.SemanticUsed)
	assert.False(t, metrics.KeywordFallback)
}

func (m *mockVectorStore) Ping(_ context.Context) error { return nil }

func (m *mockVectorStore) Upsert(_ context.Context, pts []vector.Point) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.points = append(m.points, pts...)
	return nil
}

func (m *mockVectorStore) Search(_ context.Context, _ []float32, limit int) ([]vector.SearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var results []vector.SearchResult
	for i, p := range m.points {
		if len(results) >= limit {
			break
		}
		results = append(results, vector.SearchResult{
			ID:      p.ID,
			Score:   1.0 - float32(i)*0.1,
			Payload: p.Payload,
		})
	}
	return results, nil
}

func (m *mockVectorStore) Delete(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	deleteSet := map[string]bool{}
	for _, id := range ids {
		deleteSet[id] = true
	}
	var kept []vector.Point
	for _, p := range m.points {
		if !deleteSet[p.ID] {
			kept = append(kept, p)
		}
	}
	m.points = kept
	return nil
}

func (m *mockVectorStore) RecreateCollection(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.points = nil
	return nil
}

func setupTestVault(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "context-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	vaultRoot := filepath.Join(dir, ".agent-vault")
	err = os.MkdirAll(vaultRoot, 0755)
	require.NoError(t, err)
	return vault.New(vaultRoot), vaultRoot
}

func embedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/embed" {
		var req struct {
			Text string `json:"text"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(map[string]any{
			"vector": []float32{0.1, 0.2, 0.3},
		})
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func TestSearcher_SearchWithEmbeddings(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("note1", "semantic content about API design", map[string]any{"type": "architecture"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	err = mvs.Upsert(context.Background(), []vector.Point{
		{
			ID:     "note1:0",
			Vector: []float32{0.1, 0.2, 0.3},
			Payload: map[string]any{
				"note_path": "note1",
				"chunk_idx": 0,
			},
		},
	})
	require.NoError(t, err)

	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	results, err := s.Search(context.Background(), "API design", 5, "")
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	assert.Equal(t, "note1", results[0].Note.Path)
	assert.Equal(t, "architecture", results[0].Note.Metadata["type"])
	assert.Greater(t, results[0].Score, float32(0))
}

func TestSearcher_SearchWithEmbeddings_FallbackToKeyword(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("note1", "this matches the keyword query", map[string]any{"type": "task"})
	require.NoError(t, err)
	_, err = v2.CreateNote("note2", "no match here", map[string]any{"type": "bug"})
	require.NoError(t, err)

	mvs := &mockVectorStore{}

	s := pkgctx.NewSearcher(v, mvs, "http://127.0.0.1:1")
	results, err := s.Search(context.Background(), "keyword query", 5, "")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "note1", results[0].Note.Path)
	assert.Equal(t, float32(0.5), results[0].Score)
}

func TestSearcher_SearchWithTaskTypeFilter(t *testing.T) {
	v, vaultRoot := setupTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("arch-note", "REST API design principles", map[string]any{"type": "architecture"})
	require.NoError(t, err)
	_, err = v2.CreateNote("bug-note", "bug in the API handler", map[string]any{"type": "bug"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	err = mvs.Upsert(context.Background(), []vector.Point{
		{
			ID:     "arch-note:0",
			Vector: []float32{0.1, 0.2, 0.3},
			Payload: map[string]any{
				"note_path": "arch-note",
				"chunk_idx": 0,
			},
		},
		{
			ID:     "bug-note:0",
			Vector: []float32{0.4, 0.5, 0.6},
			Payload: map[string]any{
				"note_path": "bug-note",
				"chunk_idx": 0,
			},
		},
	})
	require.NoError(t, err)

	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)
	results, err := s.Search(context.Background(), "API", 5, "architecture")
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "arch-note", results[0].Note.Path)
}

func TestSearcher_EmptyVault(t *testing.T) {
	v, _ := setupTestVault(t)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	s := pkgctx.NewSearcher(v, mvs, embedSrv.URL)

	results, err := s.Search(context.Background(), "something", 5, "")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearcherHybridFusesLexicalAndSemanticAndCachesQueryEmbedding(t *testing.T) {
	v, _ := setupTestVault(t)
	_, err := v.CreateNote("semantic-note", "architecture overview", map[string]any{"type": "architecture"})
	require.NoError(t, err)
	_, err = v.CreateNote("lexical-note", "login authorization failure and recovery", map[string]any{"type": "bug"})
	require.NoError(t, err)
	var embeddingCalls atomic.Int32
	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embeddingCalls.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"vector": []float32{.1, .2, .3}})
	}))
	defer embedSrv.Close()
	store := &mockVectorStore{points: []vector.Point{{ID: "semantic-note:0", Vector: []float32{.1, .2, .3}, Payload: map[string]any{"note_path": "semantic-note", "content": "architecture overview"}}}}
	searcher := pkgctx.NewSearcher(v, store, embedSrv.URL)

	results, metrics, err := searcher.SearchWithMetrics(context.Background(), "login authorization", 5, "")
	require.NoError(t, err)
	require.Len(t, results, 2)
	paths := []string{results[0].Note.Path, results[1].Note.Path}
	assert.Contains(t, paths, "semantic-note")
	assert.Contains(t, paths, "lexical-note")
	assert.NotEmpty(t, results[0].Reason)
	assert.False(t, metrics.QueryEmbeddingCacheHit)
	assert.True(t, metrics.SemanticUsed)
	assert.Greater(t, metrics.LexicalCandidates, 0)

	_, metrics, err = searcher.SearchWithMetrics(context.Background(), "login authorization", 5, "")
	require.NoError(t, err)
	assert.True(t, metrics.QueryEmbeddingCacheHit)
	assert.Equal(t, int32(1), embeddingCalls.Load())
}
