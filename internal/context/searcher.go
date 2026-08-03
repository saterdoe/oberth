package context

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/vector"
)

// Searcher performs semantic search over the vault.
type Searcher struct {
	vault       *vault.Vault
	vectorStore vector.VectorStore
	embedder    Embedder
	storeMu     sync.RWMutex
	cacheMu     sync.Mutex
	queryCache  map[string]queryEmbeddingEntry
}

// Embedder exposes the configured embedding contract to repository-scoped
// indexes without coupling them to the Vault vector collection.
func (s *Searcher) Embedder() Embedder {
	if s == nil {
		return nil
	}
	return s.embedder
}

type queryEmbeddingEntry struct {
	vector    []float32
	expiresAt time.Time
	usedAt    time.Time
}

// SearchResult represents a search result with a note and relevance score.
type SearchResult struct {
	Note          vault.Note `json:"note"`
	Score         float32    `json:"score"`
	Excerpt       string     `json:"excerpt,omitempty"`
	Reason        string     `json:"reason,omitempty"`
	SemanticScore float32    `json:"semantic_score,omitempty"`
	LexicalScore  float32    `json:"lexical_score,omitempty"`
}

type RetrievalMetrics struct {
	Mode                   string `json:"mode"`
	SemanticUsed           bool   `json:"semantic_used"`
	KeywordFallback        bool   `json:"keyword_fallback"`
	QueryEmbeddingCacheHit bool   `json:"query_embedding_cache_hit"`
	VectorCandidates       int    `json:"vector_candidates"`
	LexicalCandidates      int    `json:"lexical_candidates"`
	FusedCandidates        int    `json:"fused_candidates"`
}

// NewSearcher creates a new Searcher.
func NewSearcher(v *vault.Vault, vs vector.VectorStore, embedURL string) *Searcher {
	return NewSearcherWithEmbedder(v, vs, NewHTTPEmbedder(embedURL, 384))
}

func NewSearcherWithEmbedder(v *vault.Vault, vs vector.VectorStore, embedder Embedder) *Searcher {
	return &Searcher{
		vault:       v,
		vectorStore: vs,
		embedder:    embedder,
		queryCache:  map[string]queryEmbeddingEntry{},
	}
}

// Ping verifies that the configured vector store is reachable.
func (s *Searcher) Ping(ctx context.Context) error {
	if s == nil {
		return errors.New("vector store is not configured")
	}
	s.storeMu.RLock()
	store := s.vectorStore
	s.storeMu.RUnlock()
	if store == nil {
		return errors.New("vector store is not configured")
	}
	return store.Ping(ctx)
}

func (s *Searcher) SetVectorStore(store vector.VectorStore) {
	if s == nil {
		return
	}
	s.storeMu.Lock()
	s.vectorStore = store
	s.storeMu.Unlock()
}

// Search finds the top-K notes most semantically similar to the query.
// If the embeddings service is unavailable, falls back to keyword search.
// Returns notes sorted by relevance score descending.
func (s *Searcher) Search(ctx context.Context, query string, limit int, taskType string) ([]SearchResult, error) {
	results, _, err := s.SearchWithMetrics(ctx, query, limit, taskType)
	return results, err
}

// SearchWithMetrics combines semantic and lexical ranks through reciprocal
// rank fusion. It keeps retrieval useful when either embeddings or exact terms
// are weak and exposes the cache/fallback path for token-economy telemetry.
func (s *Searcher) SearchWithMetrics(ctx context.Context, query string, limit int, taskType string) ([]SearchResult, RetrievalMetrics, error) {
	metrics := RetrievalMetrics{}
	if limit <= 0 {
		limit = 5
	}
	lexical, err := s.lexicalSearch(query, limit*3, taskType)
	if err != nil {
		return nil, metrics, err
	}
	metrics.LexicalCandidates = len(lexical)
	s.storeMu.RLock()
	store := s.vectorStore
	s.storeMu.RUnlock()
	if store == nil || s.embedder == nil {
		semantic, semanticErr := s.localSemanticSearch(query, limit*3, taskType)
		if semanticErr != nil {
			metrics.Mode = "keyword"
			metrics.KeywordFallback = true
			markKeywordFallback(lexical)
			if len(lexical) > limit {
				lexical = lexical[:limit]
			}
			return lexical, metrics, nil
		}
		metrics.SemanticUsed = true
		metrics.Mode = "local-trigram"
		metrics.VectorCandidates = len(semantic)
		fused := fuseRetrievalRanks(semantic, lexical)
		metrics.FusedCandidates = len(fused)
		if len(fused) > limit {
			fused = fused[:limit]
		}
		return fused, metrics, nil
	}
	vec, cacheHit, err := s.embedQueryCached(ctx, query)
	metrics.QueryEmbeddingCacheHit = cacheHit
	if err != nil {
		metrics.Mode = "keyword"
		slog.Warn("embeddings service unavailable, falling back to keyword search", "error", err)
		metrics.KeywordFallback = true
		markKeywordFallback(lexical)
		if len(lexical) > limit {
			lexical = lexical[:limit]
		}
		return lexical, metrics, nil
	}

	results, err := store.Search(ctx, vec, limit)
	if err != nil {
		metrics.Mode = "keyword"
		metrics.KeywordFallback = true
		markKeywordFallback(lexical)
		if len(lexical) > limit {
			lexical = lexical[:limit]
		}
		return lexical, metrics, nil
	}
	metrics.SemanticUsed = true
	metrics.Mode = "vector"
	metrics.VectorCandidates = len(results)
	seen := make(map[string]bool)
	var semantic []SearchResult

	for _, r := range results {
		notePath, _ := r.Payload["note_path"].(string)
		if strings.TrimSpace(notePath) == "" {
			notePath = extractNotePath(r.ID)
		}
		if seen[notePath] {
			continue
		}
		seen[notePath] = true

		note, err := s.vault.ReadNote(notePath)
		if err != nil {
			slog.Warn("failed to read note from vault", "id", notePath, "error", err)
			continue
		}
		if taskType != "" {
			nt, ok := note.Metadata["type"].(string)
			if !ok || nt != taskType {
				continue
			}
		}
		excerpt, _ := r.Payload["content"].(string)
		if strings.TrimSpace(excerpt) == "" {
			excerpt = relevantExcerpt(note.Content, query, 2400)
		}
		semantic = append(semantic, SearchResult{Note: *note, Score: r.Score, SemanticScore: r.Score, Excerpt: excerpt, Reason: "semantic vector match"})
	}
	fused := fuseRetrievalRanks(semantic, lexical)
	metrics.FusedCandidates = len(fused)
	if len(fused) > limit {
		fused = fused[:limit]
	}
	return fused, metrics, nil
}

// localSemanticSearch provides a private, dependency-free semantic baseline.
// Character n-grams capture related identifiers and word morphology without
// sending code or memory outside the machine. Configured embedding/vector
// providers still take precedence when available.
func (s *Searcher) localSemanticSearch(query string, limit int, taskType string) ([]SearchResult, error) {
	notes, err := s.vault.ListAllNotes()
	if err != nil {
		if errors.Is(err, vault.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	queryVector := trigramVector(query)
	results := make([]SearchResult, 0)
	for _, note := range notes {
		if taskType != "" {
			noteType, _ := note.Metadata["type"].(string)
			if noteType != taskType {
				continue
			}
		}
		score := cosineSimilarity(queryVector, trigramVector(note.Path+"\n"+note.Content))
		if score < 0.08 {
			continue
		}
		results = append(results, SearchResult{
			Note: note, Score: score, SemanticScore: score,
			Excerpt: relevantExcerpt(note.Content, query, 2400), Reason: "private local semantic similarity",
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func trigramVector(value string) map[string]float32 {
	normalized := " " + strings.Join(strings.Fields(strings.ToLower(value)), " ") + " "
	result := map[string]float32{}
	runes := []rune(normalized)
	for index := 0; index+3 <= len(runes); index++ {
		result[string(runes[index:index+3])]++
	}
	return result
}

func cosineSimilarity(left, right map[string]float32) float32 {
	var dot, leftNorm, rightNorm float32
	for key, value := range left {
		leftNorm += value * value
		dot += value * right[key]
	}
	for _, value := range right {
		rightNorm += value * value
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / float32(math.Sqrt(float64(leftNorm*rightNorm)))
}

func markKeywordFallback(results []SearchResult) {
	for index := range results {
		results[index].Score = 0.5
		results[index].LexicalScore = 0.5
		results[index].Reason = "keyword fallback"
	}
}

type embedRequest struct {
	Text string `json:"text"`
}

func (s *Searcher) embedQuery(ctx context.Context, query string) ([]float32, error) {
	if s.embedder == nil {
		return nil, errors.New("embedder is not configured")
	}
	return s.embedder.Embed(ctx, query)
}

func (s *Searcher) embedQueryCached(ctx context.Context, query string) ([]float32, bool, error) {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(query)))))
	now := time.Now()
	s.cacheMu.Lock()
	if entry, ok := s.queryCache[key]; ok && now.Before(entry.expiresAt) {
		entry.usedAt = now
		s.queryCache[key] = entry
		vectorCopy := append([]float32(nil), entry.vector...)
		s.cacheMu.Unlock()
		return vectorCopy, true, nil
	}
	delete(s.queryCache, key)
	s.cacheMu.Unlock()
	vec, err := s.embedQuery(ctx, query)
	if err != nil {
		return nil, false, err
	}
	s.cacheMu.Lock()
	if len(s.queryCache) >= 256 {
		var oldestKey string
		var oldest time.Time
		for candidate, entry := range s.queryCache {
			if oldestKey == "" || entry.usedAt.Before(oldest) {
				oldestKey, oldest = candidate, entry.usedAt
			}
		}
		delete(s.queryCache, oldestKey)
	}
	s.queryCache[key] = queryEmbeddingEntry{vector: append([]float32(nil), vec...), expiresAt: now.Add(10 * time.Minute), usedAt: now}
	s.cacheMu.Unlock()
	return vec, false, nil
}

func (s *Searcher) keywordSearch(ctx context.Context, query string, limit int, taskType string) ([]SearchResult, error) {
	results, err := s.lexicalSearch(query, limit, taskType)
	if err != nil {
		return nil, err
	}
	for index := range results {
		results[index].Score = 0.5
		results[index].LexicalScore = 0.5
	}
	return results, nil
}

func (s *Searcher) lexicalSearch(query string, limit int, taskType string) ([]SearchResult, error) {
	notes, err := s.vault.ListAllNotes()
	if err != nil {
		if errors.Is(err, vault.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	terms := queryTermsForRanking(query)
	var results []SearchResult

	for _, note := range notes {
		if taskType != "" {
			nt, ok := note.Metadata["type"].(string)
			if !ok || nt != taskType {
				continue
			}
		}
		lower := strings.ToLower(note.Path + "\n" + note.Content)
		matches := 0
		for _, term := range terms {
			matches += strings.Count(lower, term)
		}
		if matches > 0 {
			score := float32(matches)
			results = append(results, SearchResult{Note: note, Score: score, LexicalScore: score, Excerpt: relevantExcerpt(note.Content, query, 2400), Reason: "lexical term match"})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func fuseRetrievalRanks(semantic, lexical []SearchResult) []SearchResult {
	type fusedItem struct {
		result           SearchResult
		score            float32
		semRank, lexRank int
	}
	items := map[string]*fusedItem{}
	for rank, result := range semantic {
		item := items[result.Note.Path]
		if item == nil {
			copyResult := result
			item = &fusedItem{result: copyResult}
			items[result.Note.Path] = item
		}
		item.score += 1 / float32(60+rank+1)
		item.semRank = rank + 1
		item.result.SemanticScore = result.Score
		if item.result.Excerpt == "" {
			item.result.Excerpt = result.Excerpt
		}
	}
	for rank, result := range lexical {
		item := items[result.Note.Path]
		if item == nil {
			copyResult := result
			item = &fusedItem{result: copyResult}
			items[result.Note.Path] = item
		}
		item.score += 1 / float32(60+rank+1)
		item.lexRank = rank + 1
		item.result.LexicalScore = result.Score
		if item.result.Excerpt == "" {
			item.result.Excerpt = result.Excerpt
		}
	}
	fused := make([]SearchResult, 0, len(items))
	for _, item := range items {
		item.result.Score = item.score
		switch {
		case item.semRank > 0 && item.lexRank > 0:
			item.result.Reason = fmt.Sprintf("hybrid fusion: semantic rank %d + lexical rank %d", item.semRank, item.lexRank)
		case item.semRank > 0:
			item.result.Reason = fmt.Sprintf("semantic rank %d", item.semRank)
		default:
			item.result.Reason = fmt.Sprintf("lexical rank %d", item.lexRank)
		}
		fused = append(fused, item.result)
	}
	sort.SliceStable(fused, func(i, j int) bool {
		if fused[i].Score == fused[j].Score {
			return fused[i].Note.Path < fused[j].Note.Path
		}
		return fused[i].Score > fused[j].Score
	})
	return fused
}

func relevantExcerpt(content, query string, maxChars int) string {
	content = strings.TrimSpace(content)
	if len(content) <= maxChars {
		return content
	}
	lower := strings.ToLower(content)
	position := -1
	for _, term := range queryTermsForRanking(query) {
		if found := strings.Index(lower, term); found >= 0 && (position < 0 || found < position) {
			position = found
		}
	}
	if position < 0 {
		return content[:maxChars]
	}
	start := position - maxChars/3
	if start < 0 {
		start = 0
	}
	end := start + maxChars
	if end > len(content) {
		end = len(content)
		start = end - maxChars
		if start < 0 {
			start = 0
		}
	}
	return content[start:end]
}

func extractNotePath(chunkID string) string {
	if idx := strings.LastIndex(chunkID, ":"); idx >= 0 {
		return chunkID[:idx]
	}
	return chunkID
}
