package context_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func embedBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/embed-batch" {
		var req struct {
			Texts []string `json:"texts"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		vectors := make([][]float32, len(req.Texts))
		for i := range vectors {
			vectors[i] = []float32{0.1, 0.2, 0.3}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"vectors": vectors,
		})
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

type indexerTestVault struct {
	root string
}

func setupIndexerTestVault(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "indexer-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	vaultRoot := filepath.Join(dir, ".agent-vault")
	err = os.MkdirAll(vaultRoot, 0755)
	require.NoError(t, err)
	return vault.New(vaultRoot), vaultRoot
}

func TestIndexer_ReindexWithNotes(t *testing.T) {
	v, vaultRoot := setupIndexerTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("note1", "content about architecture", map[string]any{"type": "architecture"})
	require.NoError(t, err)
	_, err = v2.CreateNote("note2", "content about bugs", map[string]any{"type": "bug"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedBatchHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	idx := pkgctx.NewIndexer(v, mvs, embedSrv.URL)

	result, err := idx.Reindex(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalNotes)
	assert.Greater(t, result.IndexedChunks, 0)
	assert.Greater(t, result.DurationMs, int64(0))

	mvs.mu.Lock()
	defer mvs.mu.Unlock()
	assert.NotEmpty(t, mvs.points)
	assert.Contains(t, mvs.points[0].ID, "note1")
}

func TestIndexer_ReindexEmptyVault(t *testing.T) {
	v, _ := setupIndexerTestVault(t)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedBatchHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	idx := pkgctx.NewIndexer(v, mvs, embedSrv.URL)

	result, err := idx.Reindex(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalNotes)
	assert.Equal(t, 0, result.IndexedChunks)
	assert.GreaterOrEqual(t, result.DurationMs, int64(0))
}

func TestIndexerMigrationBuildsTargetBeforeSwitch(t *testing.T) {
	v, root := setupIndexerTestVault(t)
	_, err := vault.New(root).CreateNote("architecture", "portable semantic index", nil)
	require.NoError(t, err)
	source := &mockVectorStore{}
	target := &mockVectorStore{}
	indexer := pkgctx.NewIndexerWithEmbedder(v, source, pkgctx.NewBuiltinEmbedder(384))
	require.NoError(t, func() error { _, err := indexer.Reindex(context.Background()); return err }())

	result, err := indexer.Migrate(context.Background(), target)
	require.NoError(t, err)
	require.Positive(t, result.IndexedChunks)
	target.mu.Lock()
	require.NotEmpty(t, target.points)
	target.mu.Unlock()
}

func TestIndexer_ReindexIncrementalNoChanges(t *testing.T) {
	v, vaultRoot := setupIndexerTestVault(t)
	v2 := vault.New(vaultRoot)
	_, err := v2.CreateNote("note1", "some content", map[string]any{"type": "task"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedBatchHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	idx := pkgctx.NewIndexer(v, mvs, embedSrv.URL)

	result1, err := idx.Reindex(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, result1.TotalNotes)

	before := len(mvs.points)

	result2, err := idx.ReindexIncremental(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, result2.TotalNotes)
	assert.Equal(t, 0, result2.IndexedChunks)

	mvs.mu.Lock()
	after := len(mvs.points)
	mvs.mu.Unlock()
	assert.Equal(t, before, after)
}

func TestIndexer_ReindexIncrementalDeletesStaleChunks(t *testing.T) {
	v, vaultRoot := setupIndexerTestVault(t)
	v2 := vault.New(vaultRoot)

	longContent := strings.Repeat("architecture decision ", 900)
	_, err := v2.CreateNote("note1", longContent, map[string]any{"type": "architecture"})
	require.NoError(t, err)

	embedSrv := httptest.NewServer(http.HandlerFunc(embedBatchHandler))
	defer embedSrv.Close()

	mvs := &mockVectorStore{}
	idx := pkgctx.NewIndexer(v, mvs, embedSrv.URL)

	result1, err := idx.Reindex(context.Background())
	require.NoError(t, err)
	require.Greater(t, result1.IndexedChunks, 1)

	_, err = v2.UpdateNote("note1", "short replacement", map[string]any{"type": "architecture"})
	require.NoError(t, err)

	result2, err := idx.ReindexIncremental(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result2.TotalNotes)
	require.Equal(t, 1, result2.IndexedChunks)

	mvs.mu.Lock()
	defer mvs.mu.Unlock()
	require.Len(t, mvs.points, 1)
	assert.Equal(t, "note1:0", mvs.points[0].ID)
}
