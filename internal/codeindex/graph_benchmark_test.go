package codeindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const codeMapPerformanceFixtureFiles = 1000

func BenchmarkCodeMapNeighborhood1000Files(b *testing.B) {
	index, seed := codeMapPerformanceFixture(b, codeMapPerformanceFixtureFiles)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		result, err := index.GraphNeighborhood(seed, GraphDirectionBoth, 100, "")
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Edges) != 5 {
			b.Fatalf("unexpected relationship count: %d", len(result.Edges))
		}
	}
}

func TestCodeMapQueryPerformanceBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("performance fixture disabled in short mode")
	}
	index, seed := codeMapPerformanceFixture(t, codeMapPerformanceFixtureFiles)
	started := time.Now()
	for n := 0; n < 100; n++ {
		result, err := index.GraphNeighborhood(seed, GraphDirectionBoth, 100, "")
		require.NoError(t, err)
		require.Len(t, result.Edges, 5)
	}
	average := time.Since(started) / 100
	require.Less(t, average, 5*time.Millisecond, "one-hop backend query exceeded its local budget")

	allocations := testing.AllocsPerRun(20, func() {
		_, err := index.GraphNeighborhood(seed, GraphDirectionBoth, 100, "")
		if err != nil {
			t.Fatal(err)
		}
	})
	require.Less(t, allocations, float64(10000), "one-hop query allocation count regressed")
}

type fixtureTesting interface {
	Helper()
	TempDir() string
	Fatal(args ...any)
}

func codeMapPerformanceFixture(t fixtureTesting, count int) (*Index, string) {
	t.Helper()
	root := t.TempDir()
	for n := 0; n < count; n++ {
		imports := ""
		if n > 0 {
			imports += fmt.Sprintf("import './file-%04d'\n", n-1)
		}
		if n+1 < count {
			imports += fmt.Sprintf("import './file-%04d'\n", n+1)
		}
		writePerformanceFixture(t, root, fmt.Sprintf("src/file-%04d.ts", n), imports+fmt.Sprintf("export const value%d = %d\n", n, n))
	}
	index, err := Open(root, filepath.Join(t.TempDir(), "index"), nil, nil, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = index.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	seed := graphNodeID(index.state.RepoID, GraphNodeFile, "src/file-0500.ts")
	return index, seed
}

func writePerformanceFixture(t fixtureTesting, root, relative, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
