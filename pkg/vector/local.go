package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// LocalStore is a small, exact vector index persisted as a single JSON file.
// It is intended for desktop vaults where zero-configuration and portability
// are more important than approximate search at very large scale.
type LocalStore struct {
	path       string
	vectorSize int
	mu         sync.RWMutex
	points     map[string]Point
}

func NewLocalStore(path string, vectorSize int) (*LocalStore, error) {
	store := &LocalStore{path: path, vectorSize: vectorSize, points: make(map[string]Point)}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *LocalStore) Ping(context.Context) error { return nil }

func (s *LocalStore) Upsert(_ context.Context, points []Point) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, point := range points {
		if len(point.Vector) != s.vectorSize {
			return fmt.Errorf("vector %q has %d dimensions; expected %d", point.ID, len(point.Vector), s.vectorSize)
		}
		s.points[point.ID] = clonePoint(point)
	}
	return s.persistLocked()
}

func (s *LocalStore) Search(_ context.Context, query []float32, limit int) ([]SearchResult, error) {
	if len(query) != s.vectorSize {
		return nil, fmt.Errorf("query has %d dimensions; expected %d", len(query), s.vectorSize)
	}
	if limit <= 0 {
		return []SearchResult{}, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	results := make([]SearchResult, 0, len(s.points))
	for _, point := range s.points {
		results = append(results, SearchResult{
			ID: point.ID, Score: cosine(query, point.Vector), Payload: clonePayload(point.Payload),
		})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *LocalStore) Delete(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.points, id)
	}
	return s.persistLocked()
}

func (s *LocalStore) RecreateCollection(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points = make(map[string]Point)
	return s.persistLocked()
}

func (s *LocalStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.points)
}

func (s *LocalStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local vector index: %w", err)
	}
	var snapshot struct {
		Version    int     `json:"version"`
		VectorSize int     `json:"vector_size"`
		Points     []Point `json:"points"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("decode local vector index: %w", err)
	}
	if snapshot.VectorSize != 0 && snapshot.VectorSize != s.vectorSize {
		return fmt.Errorf("local vector index uses %d dimensions; configured embedder uses %d", snapshot.VectorSize, s.vectorSize)
	}
	for _, point := range snapshot.Points {
		if len(point.Vector) == s.vectorSize {
			s.points[point.ID] = clonePoint(point)
		}
	}
	return nil
}

func (s *LocalStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create local vector directory: %w", err)
	}
	points := make([]Point, 0, len(s.points))
	for _, point := range s.points {
		points = append(points, clonePoint(point))
	}
	sort.Slice(points, func(i, j int) bool { return points[i].ID < points[j].ID })
	data, err := json.Marshal(struct {
		Version    int     `json:"version"`
		VectorSize int     `json:"vector_size"`
		Points     []Point `json:"points"`
	}{Version: 1, VectorSize: s.vectorSize, Points: points})
	if err != nil {
		return fmt.Errorf("encode local vector index: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write local vector index: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace local vector index: %w", err)
	}
	return nil
}

func cosine(a, b []float32) float32 {
	var dot, normA, normB float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		normA += x * x
		normB += y * y
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

func clonePoint(point Point) Point {
	return Point{ID: point.ID, Vector: append([]float32(nil), point.Vector...), Payload: clonePayload(point.Payload)}
}

func clonePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	result := make(map[string]any, len(payload))
	for key, value := range payload {
		result[key] = value
	}
	return result
}
