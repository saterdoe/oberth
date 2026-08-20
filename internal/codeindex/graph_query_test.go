package codeindex

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphQueriesAreBoundedDeterministicAndMetadataOnly(t *testing.T) {
	root := t.TempDir()
	writeGraphFixture(t, root, "app.ts", "import './a'\nimport './b'\n")
	writeGraphFixture(t, root, "a.ts", "export const a = 'SOURCE_SECRET'\n")
	writeGraphFixture(t, root, "b.ts", "export const b = 1\n")
	index, err := Open(root, filepath.Join(t.TempDir(), "index"), nil, nil, DefaultOptions())
	require.NoError(t, err)
	_, err = index.Update(context.Background())
	require.NoError(t, err)

	first, err := index.FindGraphNodes(".ts", 1, "")
	require.NoError(t, err)
	require.Len(t, first.Nodes, 1)
	require.True(t, first.Truncated)
	require.NotEmpty(t, first.NextCursor)
	second, err := index.FindGraphNodes(".ts", 1, first.NextCursor)
	require.NoError(t, err)
	require.Len(t, second.Nodes, 1)
	require.NotEqual(t, first.Nodes[0].ID, second.Nodes[0].ID)

	app := graphNodeID(index.state.RepoID, GraphNodeFile, "app.ts")
	neighbors, err := index.GraphNeighborhood(app, GraphDirectionOutgoing, 100, "")
	require.NoError(t, err)
	require.Len(t, neighbors.Edges, 2)
	require.Len(t, neighbors.Nodes, 3)
	require.Equal(t, neighbors, mustNeighborhood(t, index, app))

	payload, err := json.Marshal(neighbors)
	require.NoError(t, err)
	require.NotContains(t, string(payload), root)
	require.NotContains(t, string(payload), "SOURCE_SECRET")
}

func TestGraphQueryRejectsMissingNodeAndStaleCursor(t *testing.T) {
	root := t.TempDir()
	writeGraphFixture(t, root, "app.ts", "export const app = 1\n")
	index, err := Open(root, filepath.Join(t.TempDir(), "index"), nil, nil, DefaultOptions())
	require.NoError(t, err)
	_, err = index.Update(context.Background())
	require.NoError(t, err)
	_, err = index.GraphNeighborhood("missing", GraphDirectionBoth, 1, "")
	require.ErrorIs(t, err, ErrGraphNodeNotFound)
	_, err = index.FindGraphNodes("", 1, encodeGraphCursor("old-fingerprint", 1))
	require.ErrorIs(t, err, ErrGraphCursorStale)
}

func mustNeighborhood(t *testing.T, index *Index, node string) GraphQueryResult {
	t.Helper()
	result, err := index.GraphNeighborhood(node, GraphDirectionOutgoing, 100, "")
	require.NoError(t, err)
	return result
}
