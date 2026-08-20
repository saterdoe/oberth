package codeindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/saterdoe/oberth/pkg/vector"
)

type Embedder interface {
	Embed(context.Context, string) ([]float32, error)
	EmbedBatch(context.Context, []string) ([][]float32, error)
	Fingerprint() string
	Dimensions() int
}
type state struct {
	Version, RepoID, Embedder string
	LastIndexed               time.Time
	Files                     map[string]string
	Chunks                    map[string]Chunk
	Graph                     GraphSnapshot
	GraphEdgesByFile          map[string][]string
}
type Index struct {
	root, statePath string
	options         Options
	embedder        Embedder
	store           vector.VectorStore
	mu              sync.RWMutex
	state           state
	status          Status
}

func Open(root, dataDir string, embedder Embedder, store vector.VectorStore, opts Options) (*Index, error) {
	return openWithIdentity(root, root, dataDir, embedder, store, opts)
}

func openWithIdentity(root, identityRoot, dataDir string, embedder Embedder, store vector.VectorStore, opts Options) (*Index, error) {
	repo, e := RepositoryID(identityRoot)
	if e != nil {
		return nil, e
	}
	if opts.MaxFiles <= 0 {
		opts = DefaultOptions()
	}
	if dataDir == "" {
		return nil, errors.New("code index data directory is required")
	}
	i := &Index{root: root, statePath: filepath.Join(dataDir, "state.json"), options: opts, embedder: embedder, store: store, state: state{Version: SchemaVersion, RepoID: repo, Files: map[string]string{}, Chunks: map[string]Chunk{}, Graph: GraphSnapshot{SchemaVersion: GraphSchemaVersion, RepoID: repo, Nodes: map[string]GraphNode{}, Edges: map[string]GraphEdge{}}, GraphEdgesByFile: map[string][]string{}}}
	if b, e := os.ReadFile(i.statePath); e == nil {
		var old state
		if json.Unmarshal(b, &old) == nil && old.Version == SchemaVersion && old.RepoID == repo && old.Embedder == fingerprint(embedder) {
			i.state = old
			if i.state.Graph.Nodes == nil {
				i.state.Graph = GraphSnapshot{SchemaVersion: GraphSchemaVersion, RepoID: repo, Nodes: map[string]GraphNode{}, Edges: map[string]GraphEdge{}}
			}
			if i.state.GraphEdgesByFile == nil {
				i.state.GraphEdgesByFile = map[string][]string{}
			}
		}
	}
	return i, nil
}
func DefaultDataDir(root string) (string, error) {
	id, e := RepositoryID(root)
	if e != nil {
		return "", e
	}
	base, e := os.UserCacheDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(base, "oberth", "code-index", strings.TrimPrefix(id, "repo:")), nil
}
func OpenLocal(root string, embedder Embedder, opts Options) (*Index, error) {
	return OpenLocalWithIdentity(root, root, embedder, opts)
}

// OpenLocalWithIdentity indexes files from root while persisting under the
// stable base repository identity. Worktree paths therefore reuse one index.
func OpenLocalWithIdentity(root, identityRoot string, embedder Embedder, opts Options) (*Index, error) {
	dir, e := DefaultDataDir(identityRoot)
	if e != nil {
		return nil, e
	}
	dims := 384
	if embedder != nil {
		dims = embedder.Dimensions()
	}
	store, e := vector.NewLocalStore(filepath.Join(dir, "vectors.json"), dims)
	if e != nil {
		return nil, e
	}
	return openWithIdentity(root, identityRoot, dir, embedder, store, opts)
}
func fingerprint(e Embedder) string {
	if e == nil {
		return "disabled"
	}
	return e.Fingerprint()
}

func (i *Index) Update(ctx context.Context) (Metrics, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	start := time.Now()
	files, e := Discover(i.root, i.options)
	if e != nil {
		return Metrics{}, e
	}
	m := Metrics{Discovered: len(files)}
	current := map[string]bool{}
	changed := map[string]bool{}
	var pendingPoints []vector.Point
	var pendingDeletes []string
	for _, f := range files {
		if ctx.Err() != nil {
			return m, ctx.Err()
		}
		current[f.Path] = true
		if i.state.Files[f.Path] == f.Hash {
			for _, c := range i.state.Chunks {
				if c.Path == f.Path {
					m.Reused++
				}
			}
			continue
		}
		changed[f.Path] = true
		chunks := ChunkFile(i.state.RepoID, f, i.options, fingerprint(i.embedder))
		if i.options.MaxChunks > 0 && len(i.state.Chunks)+len(chunks) > i.options.MaxChunks {
			m.Skipped++
			continue
		}
		old := i.idsForPath(f.Path)
		texts := make([]string, len(chunks))
		for n := range chunks {
			texts[n] = embeddingText(chunks[n])
		}
		var vectors [][]float32
		if i.embedder != nil && i.store != nil {
			vectors, e = i.embedder.EmbedBatch(ctx, texts)
			if e != nil {
				m.Failed++
				i.status.LastError = "embedding failed; lexical index remains available"
				vectors = nil
			} else if len(vectors) != len(chunks) {
				return m, errors.New("embedder returned unexpected vector count")
			}
		}
		if len(vectors) > 0 {
			points := make([]vector.Point, len(chunks))
			for n, c := range chunks {
				if len(vectors[n]) != i.embedder.Dimensions() {
					return m, fmt.Errorf("embedding dimension mismatch for %s", f.Path)
				}
				points[n] = vector.Point{ID: c.ID, Vector: vectors[n], Payload: chunkPayload(c)}
			}
			pendingPoints = append(pendingPoints, points...)
			m.EmbeddingMisses += len(points)
		}
		newIDs := map[string]bool{}
		for _, c := range chunks {
			newIDs[c.ID] = true
		}
		var obsolete []string
		for _, id := range old {
			if !newIDs[id] {
				obsolete = append(obsolete, id)
			}
		}
		if len(obsolete) > 0 && i.store != nil {
			pendingDeletes = append(pendingDeletes, obsolete...)
			m.Deleted += len(obsolete)
		}
		for _, id := range old {
			delete(i.state.Chunks, id)
		}
		for _, c := range chunks {
			i.state.Chunks[c.ID] = c
		}
		i.state.Files[f.Path] = f.Hash
		m.Indexed++
		m.Created += len(chunks)
	}
	for path := range i.state.Files {
		if !current[path] {
			changed[path] = true
			ids := i.idsForPath(path)
			if i.store != nil && len(ids) > 0 {
				pendingDeletes = append(pendingDeletes, ids...)
			}
			for _, id := range ids {
				delete(i.state.Chunks, id)
			}
			delete(i.state.Files, path)
			m.Deleted += len(ids)
		}
	}
	if i.state.Graph.SchemaVersion != GraphSchemaVersion || len(i.state.Graph.Nodes) == 0 {
		for _, file := range files {
			changed[file.Path] = true
		}
	}
	i.state.Graph = graphForFiles(i.state.RepoID, files, i.state.Graph.Nodes, i.state.Graph.Edges, i.state.GraphEdgesByFile, changed)
	i.state.GraphEdgesByFile = importEdgesByFile(i.state.Graph.Edges)
	// Persist vectors at most twice per refresh instead of rewriting the full
	// local store once per changed file.
	if i.store != nil && len(pendingPoints) > 0 {
		if e = i.store.Upsert(ctx, pendingPoints); e != nil {
			return m, e
		}
	}
	if i.store != nil && len(pendingDeletes) > 0 {
		if e = i.store.Delete(ctx, pendingDeletes); e != nil {
			return m, e
		}
	}
	i.state.LastIndexed = time.Now().UTC()
	i.state.Embedder = fingerprint(i.embedder)
	m.Duration = time.Since(start)
	i.status = Status{SchemaVersion: SchemaVersion, RepoID: i.state.RepoID, IndexedFiles: len(i.state.Files), ChunkCount: len(i.state.Chunks), LastIndexed: i.state.LastIndexed, Fresh: true, Metrics: m, LastError: i.status.LastError}
	return m, i.persist()
}

func importEdgesByFile(edges map[string]GraphEdge) map[string][]string {
	byFile := make(map[string][]string)
	for id, edge := range edges {
		if edge.Kind == GraphEdgeImports {
			byFile[edge.SourcePath] = append(byFile[edge.SourcePath], id)
		}
	}
	for filePath := range byFile {
		sort.Strings(byFile[filePath])
	}
	return byFile
}

func (i *Index) Graph() GraphSnapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return cloneGraph(i.state.Graph)
}

func cloneGraph(source GraphSnapshot) GraphSnapshot {
	copy := GraphSnapshot{SchemaVersion: source.SchemaVersion, RepoID: source.RepoID, Fingerprint: source.Fingerprint, Nodes: make(map[string]GraphNode, len(source.Nodes)), Edges: make(map[string]GraphEdge, len(source.Edges))}
	for id, node := range source.Nodes {
		copy.Nodes[id] = node
	}
	for id, edge := range source.Edges {
		copy.Edges[id] = edge
	}
	return copy
}
func (i *Index) idsForPath(path string) []string {
	var ids []string
	for id, c := range i.state.Chunks {
		if c.Path == path {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
func (i *Index) persist() error {
	if e := os.MkdirAll(filepath.Dir(i.statePath), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(i.state)
	if e != nil {
		return e
	}
	tmp := i.statePath + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, i.statePath)
}
func (i *Index) Status() Status {
	i.mu.RLock()
	defer i.mu.RUnlock()
	s := i.status
	if s.SchemaVersion == "" {
		s = Status{SchemaVersion: SchemaVersion, RepoID: i.state.RepoID, IndexedFiles: len(i.state.Files), ChunkCount: len(i.state.Chunks), LastIndexed: i.state.LastIndexed}
	}
	s.Fresh = !s.LastIndexed.IsZero() && time.Since(s.LastIndexed) < 10*time.Minute
	return s
}
func embeddingText(c Chunk) string { return c.Path + "\n" + c.Symbol + "\n" + c.Content }
func chunkPayload(c Chunk) map[string]any {
	return map[string]any{"source": "repository", "repo_id": c.RepoID, "path": c.Path, "language": c.Language, "symbol": c.Symbol, "symbol_kind": string(c.SymbolKind), "start_line": c.StartLine, "end_line": c.EndLine, "chunk_hash": c.ChunkHash}
}
