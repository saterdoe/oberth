package codeindex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGraphExtractsGoAndStaticJSImportsWithEvidence(t *testing.T) {
	root := t.TempDir()
	writeGraphFixture(t, root, "go.mod", "module example.test/app\n")
	writeGraphFixture(t, root, "main.go", "package main\nimport (\n  \"fmt\"\n  svc \"example.test/app/internal/service\"\n)\nfunc main(){ fmt.Println(svc.Name) }\n")
	writeGraphFixture(t, root, "internal/service/service.go", "package service\nconst Name = \"service\"\n")
	writeGraphFixture(t, root, "web/app.ts", "import { value } from './lib'\nexport { other } from './other.js'\nconst pkg = require('left-pad')\nconsole.log(value, pkg)\n")
	writeGraphFixture(t, root, "web/lib.ts", "export const value = 1\n")
	writeGraphFixture(t, root, "web/other.js", "export const other = 2\n")

	index, err := Open(root, filepath.Join(t.TempDir(), "index"), nil, nil, DefaultOptions())
	require.NoError(t, err)
	_, err = index.Update(context.Background())
	require.NoError(t, err)
	graph := index.Graph()

	require.Equal(t, GraphSchemaVersion, graph.SchemaVersion)
	require.NotEmpty(t, graph.Fingerprint)
	requireGraphImport(t, graph, "main.go", "fmt", "external package")
	requireGraphImport(t, graph, "main.go", "internal/service", "resolved internal module")
	requireGraphImport(t, graph, "web/app.ts", "web/lib.ts", "resolved repository-relative import")
	requireGraphImport(t, graph, "web/app.ts", "web/other.js", "resolved repository-relative import")
	requireGraphImport(t, graph, "web/app.ts", "left-pad", "external package or unresolved alias")

	encoded, err := json.Marshal(graph)
	require.NoError(t, err)
	payload := string(encoded)
	require.NotContains(t, payload, root)
	require.NotContains(t, payload, "fmt.Println")
	require.NotContains(t, payload, "console.log")
}

func TestGraphIncrementalRefreshMatchesCleanRebuildAndRemovesGhostEdges(t *testing.T) {
	root := t.TempDir()
	writeGraphFixture(t, root, "a.ts", "import { b } from './b'\nexport const a = b\n")
	writeGraphFixture(t, root, "b.ts", "export const b = 1\n")
	data := filepath.Join(t.TempDir(), "incremental")
	index, err := Open(root, data, nil, nil, DefaultOptions())
	require.NoError(t, err)
	_, err = index.Update(context.Background())
	require.NoError(t, err)
	before := index.Graph()
	requireGraphImport(t, before, "a.ts", "b.ts", "resolved repository-relative import")

	writeGraphFixture(t, root, "a.ts", "export const a = 1\n")
	require.NoError(t, os.Remove(filepath.Join(root, "b.ts")))
	_, err = index.Update(context.Background())
	require.NoError(t, err)
	incremental := index.Graph()
	for _, edge := range incremental.Edges {
		require.NotEqual(t, GraphEdgeImports, edge.Kind, "deleted import remained as a ghost edge")
	}

	clean, err := Open(root, filepath.Join(t.TempDir(), "clean"), nil, nil, DefaultOptions())
	require.NoError(t, err)
	_, err = clean.Update(context.Background())
	require.NoError(t, err)
	cleanGraph := clean.Graph()
	require.Equal(t, cleanGraph.Fingerprint, incremental.Fingerprint)
	require.Equal(t, sortedGraphIDs(cleanGraph.Nodes), sortedGraphIDs(incremental.Nodes))
	require.Equal(t, sortedEdgeIDs(cleanGraph.Edges), sortedEdgeIDs(incremental.Edges))

	reopened, err := Open(root, data, nil, nil, DefaultOptions())
	require.NoError(t, err)
	require.Equal(t, incremental.Fingerprint, reopened.Graph().Fingerprint)
}

func TestGraphLeavesDynamicAndAmbiguousJSImportsUnresolved(t *testing.T) {
	root := t.TempDir()
	writeGraphFixture(t, root, "app.ts", "import thing from '@alias/thing'\nconst name = './dynamic'\nimport(name)\n")
	writeGraphFixture(t, root, "dynamic.ts", "export default 1\n")
	index, err := Open(root, filepath.Join(t.TempDir(), "index"), nil, nil, DefaultOptions())
	require.NoError(t, err)
	_, err = index.Update(context.Background())
	require.NoError(t, err)
	graph := index.Graph()
	requireGraphImport(t, graph, "app.ts", "@alias/thing", "external package or unresolved alias")
	for _, edge := range graph.Edges {
		if edge.Kind != GraphEdgeImports {
			continue
		}
		target := graph.Nodes[edge.TargetID]
		require.NotEqual(t, "dynamic.ts", target.Path, "dynamic import must not be represented as a static edge")
	}
}

func TestGraphSanitizesUntrustedLabelsAndBoundsImports(t *testing.T) {
	root := t.TempDir()
	imports := make([]string, 0, maxImportsPerFile+50)
	for n := 0; n < maxImportsPerFile+50; n++ {
		imports = append(imports, "import 'package-"+strconv.Itoa(n)+"'")
	}
	imports[0] = "import 'javascript:<svg onload=alert(1)>\u202e'"
	writeGraphFixture(t, root, "app.ts", strings.Join(imports, "\n"))
	index, err := Open(root, filepath.Join(t.TempDir(), "index"), nil, nil, DefaultOptions())
	require.NoError(t, err)
	_, err = index.Update(context.Background())
	require.NoError(t, err)
	graph := index.Graph()

	importCount := 0
	for _, edge := range graph.Edges {
		if edge.Kind == GraphEdgeImports {
			importCount++
		}
	}
	require.Equal(t, maxImportsPerFile, importCount)
	for _, node := range graph.Nodes {
		require.NotContains(t, node.Label, "\u202e")
		require.LessOrEqual(t, len(node.Label), maxGraphLabelBytes)
	}
}

func FuzzStaticImportExtraction(f *testing.F) {
	f.Add("import x from './x'\n", "typescript")
	f.Add("package x\nimport \"fmt\"\n", "go")
	f.Add("import '\u202ejavascript:<svg>'\n", "javascript")
	f.Fuzz(func(t *testing.T, content, language string) {
		if len(content) > 32*1024 {
			t.Skip()
		}
		if language != "go" && language != "typescript" && language != "javascript" && language != "tsx" && language != "jsx" {
			language = "typescript"
		}
		file := File{Path: "source.ts", Content: []byte(content), Language: language}
		nodes := structuralNodes("repo:fuzz", []File{file})
		edges := extractImportEdges("repo:fuzz", file, map[string]File{file.Path: file}, "", nodes)
		if len(edges) > maxImportsPerFile {
			t.Fatalf("extractor exceeded import budget: %d", len(edges))
		}
		for _, node := range nodes {
			if len(node.Label) > maxGraphLabelBytes || strings.ContainsRune(node.Label, '\u202e') {
				t.Fatalf("unsafe graph label %q", node.Label)
			}
		}
	})
}

func requireGraphImport(t *testing.T, graph GraphSnapshot, sourcePath, targetValue, resolution string) {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Kind != GraphEdgeImports || edge.SourcePath != sourcePath || edge.Resolution != resolution {
			continue
		}
		target := graph.Nodes[edge.TargetID]
		if target.Path == targetValue || target.Label == targetValue {
			require.Greater(t, edge.Range.StartLine, 0)
			require.NotEmpty(t, edge.Extractor)
			require.Equal(t, GraphConfidenceExtracted, edge.Confidence)
			return
		}
	}
	t.Fatalf("missing import %s -> %s (%s)", sourcePath, targetValue, resolution)
}

func writeGraphFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
}

func sortedGraphIDs[T any](values map[string]T) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedEdgeIDs(values map[string]GraphEdge) []string { return sortedGraphIDs(values) }
