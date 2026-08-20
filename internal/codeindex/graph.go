package codeindex

import (
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const GraphSchemaVersion = "1"

type GraphNodeKind string
type GraphEdgeKind string
type GraphConfidence string

const (
	GraphNodeRepository GraphNodeKind = "repository"
	GraphNodeDirectory  GraphNodeKind = "directory"
	GraphNodeFile       GraphNodeKind = "file"
	GraphNodeExternal   GraphNodeKind = "external_package"

	GraphEdgeContains GraphEdgeKind = "contains"
	GraphEdgeImports  GraphEdgeKind = "imports"

	GraphConfidenceExact     GraphConfidence = "exact"
	GraphConfidenceExtracted GraphConfidence = "extracted"
)

type GraphRange struct {
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column,omitempty"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column,omitempty"`
}

type GraphNode struct {
	ID            string        `json:"id"`
	RepoID        string        `json:"repo_id"`
	Kind          GraphNodeKind `json:"kind"`
	Label         string        `json:"label"`
	Path          string        `json:"path,omitempty"`
	Language      string        `json:"language,omitempty"`
	SchemaVersion string        `json:"schema_version"`
}

type GraphEdge struct {
	ID               string          `json:"id"`
	RepoID           string          `json:"repo_id"`
	SourceID         string          `json:"source_id"`
	TargetID         string          `json:"target_id"`
	Kind             GraphEdgeKind   `json:"kind"`
	SourcePath       string          `json:"source_path"`
	Range            GraphRange      `json:"range"`
	Extractor        string          `json:"extractor"`
	ExtractorVersion string          `json:"extractor_version"`
	Confidence       GraphConfidence `json:"confidence"`
	Resolution       string          `json:"resolution"`
	SchemaVersion    string          `json:"schema_version"`
}

type GraphSnapshot struct {
	SchemaVersion string               `json:"schema_version"`
	RepoID        string               `json:"repo_id"`
	Fingerprint   string               `json:"fingerprint"`
	Nodes         map[string]GraphNode `json:"nodes"`
	Edges         map[string]GraphEdge `json:"edges"`
}

func graphNodeID(repoID string, kind GraphNodeKind, value string) string {
	return "graph-node:" + hash(repoID, string(kind), value)[:32]
}

func graphEdgeID(repoID string, kind GraphEdgeKind, sourceID, targetID, sourcePath string, r GraphRange) string {
	return "graph-edge:" + hash(repoID, string(kind), sourceID, targetID, sourcePath, strconv.Itoa(r.StartLine), strconv.Itoa(r.StartColumn), strconv.Itoa(r.EndLine), strconv.Itoa(r.EndColumn))[:32]
}

func graphForFiles(repoID string, files []File, previousNodes map[string]GraphNode, previous map[string]GraphEdge, previousByFile map[string][]string, changed map[string]bool) GraphSnapshot {
	nodes := structuralNodes(repoID, files)
	edges := make(map[string]GraphEdge)
	byFile := make(map[string][]string)
	current := make(map[string]bool, len(files))
	for _, file := range files {
		current[file.Path] = true
	}
	for filePath, ids := range previousByFile {
		if changed[filePath] || !current[filePath] {
			continue
		}
		for _, id := range ids {
			if edge, ok := previous[id]; ok {
				edges[id] = edge
				byFile[filePath] = append(byFile[filePath], id)
				if target, ok := previousNodes[edge.TargetID]; ok && target.Kind == GraphNodeExternal {
					nodes[target.ID] = target
				}
			}
		}
	}

	fileSet := make(map[string]File, len(files))
	for _, f := range files {
		fileSet[f.Path] = f
	}
	module := goModulePath(fileSet["go.mod"].Content)
	for _, file := range files {
		if !changed[file.Path] {
			continue
		}
		for _, edge := range extractImportEdges(repoID, file, fileSet, module, nodes) {
			edges[edge.ID] = edge
			byFile[file.Path] = append(byFile[file.Path], edge.ID)
		}
	}
	for filePath := range byFile {
		sort.Strings(byFile[filePath])
	}
	for id, edge := range containmentEdges(repoID, files, nodes) {
		edges[id] = edge
	}
	return GraphSnapshot{SchemaVersion: GraphSchemaVersion, RepoID: repoID, Nodes: nodes, Edges: edges, Fingerprint: graphFingerprint(nodes, edges)}
}

func structuralNodes(repoID string, files []File) map[string]GraphNode {
	nodes := make(map[string]GraphNode)
	repo := GraphNode{ID: graphNodeID(repoID, GraphNodeRepository, repoID), RepoID: repoID, Kind: GraphNodeRepository, Label: "repository", SchemaVersion: GraphSchemaVersion}
	nodes[repo.ID] = repo
	for _, f := range files {
		fileNode := GraphNode{ID: graphNodeID(repoID, GraphNodeFile, f.Path), RepoID: repoID, Kind: GraphNodeFile, Label: path.Base(f.Path), Path: f.Path, Language: f.Language, SchemaVersion: GraphSchemaVersion}
		nodes[fileNode.ID] = fileNode
		for dir := path.Dir(f.Path); dir != "." && dir != "/"; dir = path.Dir(dir) {
			node := GraphNode{ID: graphNodeID(repoID, GraphNodeDirectory, dir), RepoID: repoID, Kind: GraphNodeDirectory, Label: path.Base(dir), Path: dir, SchemaVersion: GraphSchemaVersion}
			nodes[node.ID] = node
		}
	}
	return nodes
}

func containmentEdges(repoID string, files []File, nodes map[string]GraphNode) map[string]GraphEdge {
	edges := make(map[string]GraphEdge)
	repoIDNode := graphNodeID(repoID, GraphNodeRepository, repoID)
	seenDirs := make(map[string]bool)
	for _, f := range files {
		parent := path.Dir(f.Path)
		parentID := repoIDNode
		if parent != "." {
			parentID = graphNodeID(repoID, GraphNodeDirectory, parent)
		}
		fileID := graphNodeID(repoID, GraphNodeFile, f.Path)
		edge := GraphEdge{RepoID: repoID, SourceID: parentID, TargetID: fileID, Kind: GraphEdgeContains, SourcePath: f.Path, Range: GraphRange{StartLine: 1, EndLine: 1}, Extractor: "filesystem", ExtractorVersion: "1", Confidence: GraphConfidenceExact, Resolution: "filesystem parent", SchemaVersion: GraphSchemaVersion}
		edge.ID = graphEdgeID(repoID, edge.Kind, edge.SourceID, edge.TargetID, edge.SourcePath, edge.Range)
		edges[edge.ID] = edge
		for dir := parent; dir != "." && dir != "/"; dir = path.Dir(dir) {
			if seenDirs[dir] {
				continue
			}
			seenDirs[dir] = true
			grand := path.Dir(dir)
			grandID := repoIDNode
			if grand != "." {
				grandID = graphNodeID(repoID, GraphNodeDirectory, grand)
			}
			r := GraphRange{StartLine: 1, EndLine: 1}
			e := GraphEdge{RepoID: repoID, SourceID: grandID, TargetID: graphNodeID(repoID, GraphNodeDirectory, dir), Kind: GraphEdgeContains, SourcePath: dir, Range: r, Extractor: "filesystem", ExtractorVersion: "1", Confidence: GraphConfidenceExact, Resolution: "filesystem parent", SchemaVersion: GraphSchemaVersion}
			e.ID = graphEdgeID(repoID, e.Kind, e.SourceID, e.TargetID, e.SourcePath, r)
			edges[e.ID] = e
		}
	}
	return edges
}

func extractImportEdges(repoID string, file File, files map[string]File, goModule string, nodes map[string]GraphNode) []GraphEdge {
	switch file.Language {
	case "go":
		return extractGoImports(repoID, file, files, goModule, nodes)
	case "typescript", "javascript", "tsx", "jsx":
		return extractJSImports(repoID, file, files, nodes)
	default:
		return nil
	}
}

func extractGoImports(repoID string, file File, files map[string]File, module string, nodes map[string]GraphNode) []GraphEdge {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file.Path, file.Content, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	var out []GraphEdge
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath == "C" {
			continue
		}
		line := 1
		if spec.Pos().IsValid() {
			line = fset.Position(spec.Pos()).Line
		}
		targetID, resolution := resolveGoTarget(repoID, importPath, module, files, nodes)
		out = append(out, importEdge(repoID, file.Path, targetID, line, "go/parser", resolution))
	}
	return out
}

func resolveGoTarget(repoID, importPath, module string, files map[string]File, nodes map[string]GraphNode) (string, string) {
	if module != "" && (importPath == module || strings.HasPrefix(importPath, module+"/")) {
		dir := strings.TrimPrefix(strings.TrimPrefix(importPath, module), "/")
		if dir == "" {
			dir = "."
		}
		for p := range files {
			if path.Dir(p) == dir && strings.HasSuffix(p, ".go") {
				if dir == "." {
					return graphNodeID(repoID, GraphNodeRepository, repoID), "resolved internal module"
				}
				return graphNodeID(repoID, GraphNodeDirectory, dir), "resolved internal module"
			}
		}
	}
	return externalNode(repoID, importPath, nodes), "external package"
}

var jsImportRE = regexp.MustCompile(`(?m)^\s*(?:import\s+(?:[^'"\n]+?\s+from\s+)?|export\s+[^'"\n]*?\s+from\s+|(?:const|let|var)\s+[^=\n]+?=\s*require\s*\()\s*['"]([^'"]+)['"]`)

func extractJSImports(repoID string, file File, files map[string]File, nodes map[string]GraphNode) []GraphEdge {
	var out []GraphEdge
	for _, match := range jsImportRE.FindAllSubmatchIndex(file.Content, -1) {
		if len(match) < 4 {
			continue
		}
		spec := string(file.Content[match[2]:match[3]])
		targetID, resolution := resolveJSTarget(repoID, file.Path, spec, files, nodes)
		out = append(out, importEdge(repoID, file.Path, targetID, lineForOffset(file.Content, match[2]), "static-js-imports", resolution))
	}
	return out
}

func resolveJSTarget(repoID, source, spec string, files map[string]File, nodes map[string]GraphNode) (string, string) {
	if !strings.HasPrefix(spec, ".") {
		return externalNode(repoID, spec, nodes), "external package or unresolved alias"
	}
	base := path.Clean(path.Join(path.Dir(source), spec))
	candidates := []string{base, base + ".ts", base + ".tsx", base + ".js", base + ".jsx", base + ".mjs", base + ".cjs", path.Join(base, "index.ts"), path.Join(base, "index.tsx"), path.Join(base, "index.js"), path.Join(base, "index.jsx")}
	for _, candidate := range candidates {
		if _, ok := files[candidate]; ok {
			return graphNodeID(repoID, GraphNodeFile, candidate), "resolved repository-relative import"
		}
	}
	return externalNode(repoID, spec, nodes), "unresolved repository-relative import"
}

func externalNode(repoID, label string, nodes map[string]GraphNode) string {
	id := graphNodeID(repoID, GraphNodeExternal, label)
	nodes[id] = GraphNode{ID: id, RepoID: repoID, Kind: GraphNodeExternal, Label: label, SchemaVersion: GraphSchemaVersion}
	return id
}

func importEdge(repoID, sourcePath, targetID string, line int, extractor, resolution string) GraphEdge {
	r := GraphRange{StartLine: line, EndLine: line}
	edge := GraphEdge{RepoID: repoID, SourceID: graphNodeID(repoID, GraphNodeFile, sourcePath), TargetID: targetID, Kind: GraphEdgeImports, SourcePath: sourcePath, Range: r, Extractor: extractor, ExtractorVersion: "1", Confidence: GraphConfidenceExtracted, Resolution: resolution, SchemaVersion: GraphSchemaVersion}
	edge.ID = graphEdgeID(repoID, edge.Kind, edge.SourceID, edge.TargetID, edge.SourcePath, edge.Range)
	return edge
}

func goModulePath(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func lineForOffset(content []byte, offset int) int {
	if offset < 0 {
		return 1
	}
	return 1 + strings.Count(string(content[:min(offset, len(content))]), "\n")
}

func graphFingerprint(nodes map[string]GraphNode, edges map[string]GraphEdge) string {
	parts := []string{GraphSchemaVersion}
	for id := range nodes {
		parts = append(parts, "n:"+id)
	}
	for id := range edges {
		parts = append(parts, "e:"+id)
	}
	sort.Strings(parts)
	return hash(parts...)
}
