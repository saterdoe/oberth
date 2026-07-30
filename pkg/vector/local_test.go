package vector_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/saterdoe/oberth/pkg/vector"
	"github.com/stretchr/testify/require"
)

func TestLocalStorePersistsAndSearches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	store, err := vector.NewLocalStore(path, 3)
	require.NoError(t, err)
	require.NoError(t, store.Upsert(context.Background(), []vector.Point{
		{ID: "near", Vector: []float32{1, 0, 0}, Payload: map[string]any{"content": "near"}},
		{ID: "far", Vector: []float32{0, 1, 0}},
	}))

	results, err := store.Search(context.Background(), []float32{.9, .1, 0}, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "near", results[0].ID)

	reopened, err := vector.NewLocalStore(path, 3)
	require.NoError(t, err)
	require.Equal(t, 2, reopened.Count())
	require.NoError(t, reopened.Delete(context.Background(), []string{"near"}))
	require.Equal(t, 1, reopened.Count())
}

func TestLocalStoreRejectsWrongDimensions(t *testing.T) {
	store, err := vector.NewLocalStore(filepath.Join(t.TempDir(), "index.json"), 3)
	require.NoError(t, err)
	err = store.Upsert(context.Background(), []vector.Point{{ID: "bad", Vector: []float32{1, 2}}})
	require.ErrorContains(t, err, "expected 3")
}
