package context_test

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/stretchr/testify/require"
)

type countingEmbedder struct {
	calls int
	next  pkgctx.Embedder
}

func (e *countingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.calls++
	return e.next.Embed(ctx, text)
}
func (e *countingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls++
	return e.next.EmbedBatch(ctx, texts)
}
func (e *countingEmbedder) Fingerprint() string { return e.next.Fingerprint() }
func (e *countingEmbedder) Dimensions() int     { return e.next.Dimensions() }

func TestBuiltinEmbedderIsDeterministicAndNormalized(t *testing.T) {
	embedder := pkgctx.NewBuiltinEmbedder(384)
	first, err := embedder.Embed(context.Background(), "semantic architecture")
	require.NoError(t, err)
	second, err := embedder.Embed(context.Background(), "semantic architecture")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Len(t, first, 384)
	require.NotEqual(t, make([]float32, 384), first)
}

func TestCachedEmbedderReusesVectorsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "embeddings.json")
	source := &countingEmbedder{next: pkgctx.NewBuiltinEmbedder(32)}
	cache, err := pkgctx.NewCachedEmbedder(source, path)
	require.NoError(t, err)
	_, err = cache.EmbedBatch(context.Background(), []string{"one", "two"})
	require.NoError(t, err)
	require.Equal(t, 1, source.calls)

	reopenedSource := &countingEmbedder{next: pkgctx.NewBuiltinEmbedder(32)}
	reopened, err := pkgctx.NewCachedEmbedder(reopenedSource, path)
	require.NoError(t, err)
	_, err = reopened.EmbedBatch(context.Background(), []string{"one", "two"})
	require.NoError(t, err)
	require.Zero(t, reopenedSource.calls)
}
