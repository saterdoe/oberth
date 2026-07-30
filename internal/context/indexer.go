package context

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/vector"
)

// Indexer rebuilds the vector index from vault contents.
type Indexer struct {
	vault       *vault.Vault
	vectorStore vector.VectorStore
	embedder    Embedder
	mu          sync.Mutex
	operationMu sync.Mutex
	indexed     map[string]indexedNote
	lastIndexed time.Time
}

type indexedNote struct {
	Hash     string
	PointIDs []string
}

// IndexResult contains statistics from an indexing operation.
type IndexResult struct {
	TotalNotes    int   `json:"total_notes"`
	IndexedChunks int   `json:"indexed_chunks"`
	DurationMs    int64 `json:"duration_ms"`
}

// NewIndexer creates a new Indexer.
func NewIndexer(v *vault.Vault, vs vector.VectorStore, embedURL string) *Indexer {
	return NewIndexerWithEmbedder(v, vs, NewHTTPEmbedder(embedURL, 384))
}

func NewIndexerWithEmbedder(v *vault.Vault, vs vector.VectorStore, embedder Embedder) *Indexer {
	return &Indexer{
		vault:       v,
		vectorStore: vs,
		embedder:    embedder,
		indexed:     make(map[string]indexedNote),
	}
}

// Reindex rebuilds the entire vector index from the vault.
// 1. Lists all notes in the vault
// 2. For each note, chunks the content
// 3. Sends chunks to embeddings service
// 4. Upserts vectors to vector store
func (idx *Indexer) Reindex(ctx context.Context) (*IndexResult, error) {
	idx.operationMu.Lock()
	defer idx.operationMu.Unlock()
	return idx.reindexStore(ctx, idx.vectorStore)
}

func (idx *Indexer) reindexStore(ctx context.Context, store vector.VectorStore) (*IndexResult, error) {
	start := time.Now()

	notes, err := idx.listAllNotes()
	if err != nil {
		return nil, err
	}

	if err := store.RecreateCollection(ctx); err != nil {
		return nil, fmt.Errorf("recreate collection: %w", err)
	}

	result := &IndexResult{TotalNotes: len(notes)}
	builtIndex := make(map[string]indexedNote, len(notes))

	for _, note := range notes {
		chunks := chunkTextForPath(note.Path, note.Content, 512, 50)
		if len(chunks) == 0 {
			builtIndex[note.Path] = indexedNote{Hash: contentHash(note.Content)}
			continue
		}

		vectors, err := idx.embedBatch(ctx, chunks)
		if err != nil {
			return nil, fmt.Errorf("embed chunks for %s: %w", note.Path, err)
		}

		points := make([]vector.Point, len(vectors))
		for i, v := range vectors {
			points[i] = vector.Point{
				ID:     fmt.Sprintf("%s:%d", note.Path, i),
				Vector: v,
				Payload: map[string]any{
					"note_path":    note.Path,
					"chunk_idx":    i,
					"content":      chunks[i],
					"content_hash": contentHash(note.Content),
					"chunk_hash":   contentHash(chunks[i]),
				},
			}
		}

		if err := store.Upsert(ctx, points); err != nil {
			return nil, fmt.Errorf("upsert vectors for %s: %w", note.Path, err)
		}

		result.IndexedChunks += len(points)

		builtIndex[note.Path] = indexedNote{Hash: contentHash(note.Content), PointIDs: pointIDs(points)}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	idx.mu.Lock()
	idx.indexed = builtIndex
	idx.lastIndexed = time.Now().UTC()
	idx.mu.Unlock()
	return result, nil
}

// ReindexIncremental reindexes only notes that have changed since last index.
func (idx *Indexer) ReindexIncremental(ctx context.Context) (*IndexResult, error) {
	idx.operationMu.Lock()
	defer idx.operationMu.Unlock()
	start := time.Now()

	notes, err := idx.listAllNotes()
	if err != nil {
		return nil, err
	}

	var changedNotes []vault.Note
	currentPaths := make(map[string]bool, len(notes))
	idx.mu.Lock()
	for _, note := range notes {
		currentPaths[note.Path] = true
		hash := contentHash(note.Content)
		prev, ok := idx.indexed[note.Path]
		if ok && prev.Hash == hash {
			continue
		}
		changedNotes = append(changedNotes, note)
	}
	var deletedPaths []string
	for path := range idx.indexed {
		if !currentPaths[path] {
			deletedPaths = append(deletedPaths, path)
		}
	}
	idx.mu.Unlock()

	for _, path := range deletedPaths {
		if err := idx.deletePreviousPoints(ctx, path); err != nil {
			return nil, err
		}
		idx.mu.Lock()
		delete(idx.indexed, path)
		idx.mu.Unlock()
	}

	result := &IndexResult{TotalNotes: len(changedNotes)}

	for _, note := range changedNotes {
		chunks := chunkTextForPath(note.Path, note.Content, 512, 50)
		if len(chunks) == 0 {
			if err := idx.deletePreviousPoints(ctx, note.Path); err != nil {
				return nil, err
			}
			idx.rememberIndexed(note.Path, note.Content, nil)
			continue
		}

		vectors, err := idx.embedBatch(ctx, chunks)
		if err != nil {
			slog.Warn("failed to embed chunks for note", "path", note.Path, "error", err)
			return nil, err
		}

		if err := idx.deletePreviousPoints(ctx, note.Path); err != nil {
			return nil, err
		}

		points := make([]vector.Point, len(vectors))
		for i, v := range vectors {
			points[i] = vector.Point{
				ID:     fmt.Sprintf("%s:%d", note.Path, i),
				Vector: v,
				Payload: map[string]any{
					"note_path":    note.Path,
					"chunk_idx":    i,
					"content":      chunks[i],
					"content_hash": contentHash(note.Content),
					"chunk_hash":   contentHash(chunks[i]),
				},
			}
		}

		if err := idx.vectorStore.Upsert(ctx, points); err != nil {
			slog.Warn("failed to upsert vectors for note", "path", note.Path, "error", err)
			return nil, err
		}

		result.IndexedChunks += len(points)
		idx.rememberIndexed(note.Path, note.Content, pointIDs(points))
	}

	result.DurationMs = time.Since(start).Milliseconds()
	idx.mu.Lock()
	idx.lastIndexed = time.Now().UTC()
	idx.mu.Unlock()
	return result, nil
}

// Migrate fully builds and verifies a target before making it active.
func (idx *Indexer) Migrate(ctx context.Context, target vector.VectorStore) (*IndexResult, error) {
	if idx == nil || target == nil {
		return nil, errors.New("migration target is not configured")
	}
	idx.operationMu.Lock()
	defer idx.operationMu.Unlock()
	idx.mu.Lock()
	previousIndex := idx.indexed
	previousLastIndexed := idx.lastIndexed
	idx.mu.Unlock()
	result, err := idx.reindexStore(ctx, target)
	if err != nil {
		return nil, err
	}
	if err := target.Ping(ctx); err != nil {
		idx.mu.Lock()
		idx.indexed = previousIndex
		idx.lastIndexed = previousLastIndexed
		idx.mu.Unlock()
		return nil, fmt.Errorf("verify migration target: %w", err)
	}
	idx.vectorStore = target
	return result, nil
}

type IndexStatus struct {
	SchemaVersion string    `json:"schema_version"`
	IndexedFiles  int       `json:"indexed_files"`
	LastIndexed   time.Time `json:"last_indexed"`
	Fresh         bool      `json:"fresh"`
}

func (idx *Indexer) Status() IndexStatus {
	if idx == nil {
		return IndexStatus{SchemaVersion: "1"}
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return IndexStatus{
		SchemaVersion: "1", IndexedFiles: len(idx.indexed), LastIndexed: idx.lastIndexed,
		Fresh: !idx.lastIndexed.IsZero() && time.Since(idx.lastIndexed) < 10*time.Minute,
	}
}

func (idx *Indexer) rememberIndexed(path, content string, ids []string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.indexed[path] = indexedNote{Hash: contentHash(content), PointIDs: ids}
}

func (idx *Indexer) deletePreviousPoints(ctx context.Context, path string) error {
	idx.mu.Lock()
	prev := idx.indexed[path]
	idx.mu.Unlock()
	if len(prev.PointIDs) == 0 {
		return nil
	}
	if err := idx.vectorStore.Delete(ctx, prev.PointIDs); err != nil {
		return fmt.Errorf("delete previous points for %s: %w", path, err)
	}
	return nil
}

func pointIDs(points []vector.Point) []string {
	ids := make([]string, len(points))
	for i, p := range points {
		ids[i] = p.ID
	}
	return ids
}

func (idx *Indexer) listAllNotes() ([]vault.Note, error) {
	notes, err := idx.vault.ListAllNotes()
	if err != nil {
		return nil, err
	}
	return notes, nil
}

func (idx *Indexer) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if idx.embedder == nil {
		return nil, errors.New("embedder is not configured")
	}
	return idx.embedder.EmbedBatch(ctx, texts)
}

func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

func estimateTokens(text string) int {
	if len(text) == 0 {
		return 1
	}
	n := len(text) / 4
	if n < 1 {
		return 1
	}
	return n
}

// chunkText splits text into chunks using the same mixed strategy as the Python version:
//   - Short text → 1 chunk
//   - Long text → split by ## sections
//   - Very long sections → word-level split with overlap
func chunkText(text string, chunkSize, overlap int) []string {
	if text == "" {
		return nil
	}

	if estimateTokens(text) <= chunkSize {
		return []string{text}
	}

	parts := strings.Split(text, "##")
	var chunks []string

	for i, part := range parts {
		stripped := strings.TrimSpace(part)
		if stripped == "" {
			continue
		}

		var section string
		if strings.HasPrefix(text, "##") || i > 0 {
			section = "##" + part
		} else {
			section = part
		}

		if estimateTokens(section) <= chunkSize {
			chunks = append(chunks, section)
			continue
		}

		words := strings.Fields(section)
		var currentWords []string
		currentTokens := 0

		for _, word := range words {
			wordTokens := estimateTokens(word) + 1
			if currentTokens+wordTokens > chunkSize && len(currentWords) > 0 {
				chunks = append(chunks, strings.Join(currentWords, " "))

				var kept []string
				keptTokens := 0
				for j := len(currentWords) - 1; j >= 0; j-- {
					wt := estimateTokens(currentWords[j]) + 1
					if keptTokens+wt > overlap {
						break
					}
					kept = append([]string{currentWords[j]}, kept...)
					keptTokens += wt
				}

				currentWords = kept
				currentTokens = keptTokens
			}

			currentWords = append(currentWords, word)
			currentTokens += estimateTokens(word) + 1
		}

		if len(currentWords) > 0 {
			chunks = append(chunks, strings.Join(currentWords, " "))
		}
	}

	return chunks
}

// chunkTextForPath preserves syntax-level boundaries before applying the
// bounded fallback chunker. This keeps functions, classes and Markdown
// sections independently retrievable without requiring a language parser.
func chunkTextForPath(path, text string, chunkSize, overlap int) []string {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".mdx") {
		return chunkText(text, chunkSize, overlap)
	}
	markers := []string{"\nfunc ", "\ntype ", "\nclass ", "\ndef ", "\nexport function ", "\ninterface "}
	parts := []string{text}
	for _, marker := range markers {
		next := make([]string, 0, len(parts))
		for _, part := range parts {
			offset := 0
			for {
				index := strings.Index(part[offset:], marker)
				if index < 0 {
					next = append(next, part[offset:])
					break
				}
				index += offset
				if index > offset {
					next = append(next, part[offset:index])
				}
				offset = index + 1
			}
		}
		parts = next
	}
	chunks := make([]string, 0, len(parts))
	for _, part := range parts {
		chunks = append(chunks, chunkText(strings.TrimSpace(part), chunkSize, overlap)...)
	}
	return chunks
}
