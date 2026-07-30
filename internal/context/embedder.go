package context

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Embedder is independent from vector persistence so embeddings can be reused
// when an index is rebuilt or moved to another backend.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Fingerprint() string
	Dimensions() int
}

// BuiltinEmbedder provides a dependency-free local embedding baseline. It uses
// signed feature hashing over words and character trigrams, which works across
// programming languages and natural languages without network access.
type BuiltinEmbedder struct{ dimensions int }

func NewBuiltinEmbedder(dimensions int) *BuiltinEmbedder {
	if dimensions <= 0 {
		dimensions = 384
	}
	return &BuiltinEmbedder{dimensions: dimensions}
}

func (e *BuiltinEmbedder) Fingerprint() string {
	return fmt.Sprintf("pi-feature-hash-v1:%d", e.dimensions)
}
func (e *BuiltinEmbedder) Dimensions() int { return e.dimensions }
func (e *BuiltinEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vector := make([]float32, e.dimensions)
	normalized := strings.ToLower(strings.TrimSpace(text))
	features := strings.Fields(normalized)
	runes := []rune(normalized)
	for i := 0; i+2 < len(runes); i++ {
		features = append(features, string(runes[i:i+3]))
	}
	for _, feature := range features {
		sum := sha256.Sum256([]byte(feature))
		index := int(binary.LittleEndian.Uint64(sum[:8]) % uint64(e.dimensions))
		sign := float32(1)
		if sum[8]&1 == 1 {
			sign = -1
		}
		vector[index] += sign
	}
	var norm float64
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm > 0 {
		scale := float32(1 / math.Sqrt(norm))
		for i := range vector {
			vector[i] *= scale
		}
	}
	return vector, nil
}
func (e *BuiltinEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vector, err := e.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		vectors[i] = vector
	}
	return vectors, nil
}

type HTTPEmbedder struct {
	baseURL    string
	dimensions int
	client     *http.Client
}

func NewHTTPEmbedder(baseURL string, dimensions int) *HTTPEmbedder {
	return &HTTPEmbedder{baseURL: strings.TrimRight(baseURL, "/"), dimensions: dimensions, client: &http.Client{}}
}
func (e *HTTPEmbedder) Fingerprint() string {
	return fmt.Sprintf("http:%s:%d", e.baseURL, e.dimensions)
}
func (e *HTTPEmbedder) Dimensions() int { return e.dimensions }
func (e *HTTPEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	var result struct {
		Vector []float32 `json:"vector"`
	}
	if err := e.post(ctx, "/embed", embedRequest{Text: text}, &result); err != nil {
		return nil, err
	}
	return result.Vector, nil
}
func (e *HTTPEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var result struct {
		Vectors [][]float32 `json:"vectors"`
	}
	if err := e.post(ctx, "/embed-batch", map[string]any{"texts": texts}, &result); err != nil {
		return nil, err
	}
	return result.Vectors, nil
}
func (e *HTTPEmbedder) post(ctx context.Context, path string, body, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embeddings service returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

type embeddingCacheSnapshot struct {
	Version int                  `json:"version"`
	Entries map[string][]float32 `json:"entries"`
}

// CachedEmbedder persists model-compatible embeddings outside any vector DB.
type CachedEmbedder struct {
	next    Embedder
	path    string
	mu      sync.Mutex
	entries map[string][]float32
}

func NewCachedEmbedder(next Embedder, path string) (*CachedEmbedder, error) {
	cache := &CachedEmbedder{next: next, path: path, entries: make(map[string][]float32)}
	data, err := os.ReadFile(path)
	if err == nil {
		var snapshot embeddingCacheSnapshot
		if json.Unmarshal(data, &snapshot) == nil && snapshot.Entries != nil {
			cache.entries = snapshot.Entries
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read embedding cache: %w", err)
	}
	return cache, nil
}
func (e *CachedEmbedder) Fingerprint() string { return e.next.Fingerprint() }
func (e *CachedEmbedder) Dimensions() int     { return e.next.Dimensions() }
func (e *CachedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}
func (e *CachedEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	var missing []string
	var positions []int
	e.mu.Lock()
	for i, text := range texts {
		key := e.key(text)
		if cached, ok := e.entries[key]; ok {
			result[i] = append([]float32(nil), cached...)
		} else {
			missing, positions = append(missing, text), append(positions, i)
		}
	}
	e.mu.Unlock()
	if len(missing) == 0 {
		return result, nil
	}
	vectors, err := e.next.EmbedBatch(ctx, missing)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	for i, vector := range vectors {
		position := positions[i]
		result[position] = append([]float32(nil), vector...)
		e.entries[e.key(texts[position])] = append([]float32(nil), vector...)
	}
	err = e.persistLocked()
	e.mu.Unlock()
	return result, err
}
func (e *CachedEmbedder) key(text string) string {
	sum := sha256.Sum256([]byte(e.next.Fingerprint() + "\x00" + text))
	return fmt.Sprintf("%x", sum)
}
func (e *CachedEmbedder) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(embeddingCacheSnapshot{Version: 1, Entries: e.entries})
	if err != nil {
		return err
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, e.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
