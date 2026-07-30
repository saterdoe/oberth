package vector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	defaultBaseURL = "http://localhost:6334"
	defaultTimeout = 30 * time.Second
)

// QdrantStore implements VectorStore using Qdrant's REST API.
type QdrantStore struct {
	baseURL    string
	collection string
	vectorSize int
	client     *http.Client
}

// QdrantOption configures a QdrantStore.
type QdrantOption func(*QdrantStore)

// WithBaseURL sets the base URL for the Qdrant REST API.
func WithBaseURL(baseURL string) QdrantOption {
	return func(s *QdrantStore) {
		s.baseURL = baseURL
	}
}

// NewQdrantStore creates a new QdrantStore with the given collection name and vector size.
func NewQdrantStore(collection string, vectorSize int, opts ...QdrantOption) *QdrantStore {
	s := &QdrantStore{
		baseURL:    defaultBaseURL,
		collection: collection,
		vectorSize: vectorSize,
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Ping verifies that Qdrant's HTTP service is reachable.
func (s *QdrantStore) Ping(ctx context.Context) error {
	resp, err := s.doRequest(ctx, http.MethodGet, "/healthz", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Upsert inserts or updates points in the collection.
func (s *QdrantStore) Upsert(ctx context.Context, points []Point) error {
	normalized := make([]Point, len(points))
	for i, point := range points {
		normalized[i] = clonePoint(point)
		normalized[i].ID = qdrantPointID(point.ID)
	}
	body := qdrantUpsertRequest{Points: normalized}
	resp, err := s.doRequest(ctx, http.MethodPut, "/collections/"+s.collection+"/points?wait=true", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Search finds the top-K most similar vectors using the Qdrant search API.
func (s *QdrantStore) Search(ctx context.Context, vector []float32, limit int) ([]SearchResult, error) {
	body := qdrantSearchRequest{
		Vector:      vector,
		Limit:       limit,
		WithPayload: true,
	}
	resp, err := s.doRequest(ctx, http.MethodPost, "/collections/"+s.collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var searchResp qdrantSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	results := make([]SearchResult, len(searchResp.Result))
	for i, r := range searchResp.Result {
		results[i] = SearchResult{
			ID:      r.ID,
			Score:   r.Score,
			Payload: r.Payload,
		}
	}
	return results, nil
}

// Delete removes points by ID from the collection.
func (s *QdrantStore) Delete(ctx context.Context, ids []string) error {
	normalized := make([]string, len(ids))
	for i, id := range ids {
		normalized[i] = qdrantPointID(id)
	}
	body := qdrantDeleteRequest{Points: normalized}
	resp, err := s.doRequest(ctx, http.MethodPost, "/collections/"+s.collection+"/points/delete", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// qdrantPointID preserves native numeric/UUID identifiers and deterministically
// maps oberth's human-readable chunk IDs to UUIDs accepted by Qdrant.
func qdrantPointID(id string) string {
	if _, err := strconv.ParseUint(id, 10, 64); err == nil {
		return id
	}
	if parsed, err := uuid.Parse(id); err == nil {
		return parsed.String()
	}
	sum := sha256.Sum256([]byte(id))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	var generated uuid.UUID
	copy(generated[:], sum[:16])
	return generated.String()
}

// RecreateCollection drops and recreates the collection with the configured vector size.
func (s *QdrantStore) RecreateCollection(ctx context.Context) error {
	resp, err := s.doRequest(ctx, http.MethodDelete, "/collections/"+s.collection, nil)
	if err != nil {
		if !errors.Is(err, ErrCollectionNotFound) {
			return err
		}
	} else {
		resp.Body.Close()
	}

	body := qdrantCreateCollectionReq{
		Vectors: qdrantVectorsConfig{
			Size:     s.vectorSize,
			Distance: "Cosine",
		},
	}
	resp, err = s.doRequest(ctx, http.MethodPut, "/collections/"+s.collection, body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *QdrantStore) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := s.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectionFailed, err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrCollectionNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("vector store returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

type qdrantUpsertRequest struct {
	Points []Point `json:"points"`
}

type qdrantSearchRequest struct {
	Vector      []float32 `json:"vector"`
	Limit       int       `json:"limit"`
	WithPayload bool      `json:"with_payload"`
}

type qdrantSearchResp struct {
	Result []qdrantScoredPoint `json:"result"`
	Status string              `json:"status"`
	Time   float64             `json:"time"`
}

type qdrantScoredPoint struct {
	ID      string         `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload"`
}

type qdrantDeleteRequest struct {
	Points []string `json:"points"`
}

type qdrantCreateCollectionReq struct {
	Vectors qdrantVectorsConfig `json:"vectors"`
}

type qdrantVectorsConfig struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}
