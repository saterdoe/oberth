package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultChromaURL = "http://localhost:8000"

type ChromaDBStore struct {
	baseURL    string
	collection string
	vectorSize int
	client     *http.Client
}

type ChromaDBOption func(*ChromaDBStore)

func WithChromaBaseURL(baseURL string) ChromaDBOption {
	return func(s *ChromaDBStore) {
		s.baseURL = baseURL
	}
}

func NewChromaDBStore(collection string, vectorSize int, opts ...ChromaDBOption) *ChromaDBStore {
	s := &ChromaDBStore{
		baseURL:    defaultChromaURL,
		collection: collection,
		vectorSize: vectorSize,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	s.baseURL = strings.TrimRight(s.baseURL, "/")
	return s
}

// Ping verifies that ChromaDB's HTTP service is reachable.
func (s *ChromaDBStore) Ping(ctx context.Context) error {
	resp, err := s.doRequest(ctx, http.MethodGet, "/api/v1/heartbeat", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *ChromaDBStore) Upsert(ctx context.Context, points []Point) error {
	ids := make([]string, len(points))
	embeddings := make([][]float32, len(points))
	metadatas := make([]map[string]any, len(points))
	documents := make([]string, len(points))

	for i, p := range points {
		ids[i] = p.ID
		embeddings[i] = p.Vector
		metadatas[i] = p.Payload

		if content, ok := p.Payload["content"].(string); ok {
			documents[i] = content
		}
	}

	body := chromaAddRequest{
		IDs:        ids,
		Embeddings: embeddings,
		Metadatas:  metadatas,
		Documents:  documents,
	}

	resp, err := s.doRequest(ctx, http.MethodPost,
		"/api/v1/collections/"+s.collection+"/add", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *ChromaDBStore) Search(ctx context.Context, vector []float32, limit int) ([]SearchResult, error) {
	body := chromaQueryRequest{
		QueryEmbeddings: [][]float32{vector},
		NResults:        limit,
		Include:         []string{"metadatas", "documents", "distances"},
	}

	resp, err := s.doRequest(ctx, http.MethodPost,
		"/api/v1/collections/"+s.collection+"/query", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var queryResp chromaQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&queryResp); err != nil {
		return nil, fmt.Errorf("decode query response: %w", err)
	}

	var results []SearchResult
	for i := 0; i < len(queryResp.IDs); i++ {
		for j := 0; j < len(queryResp.IDs[i]); j++ {
			id := queryResp.IDs[i][j]
			var score float32
			if j < len(queryResp.Distances[i]) {
				score = 1.0 - queryResp.Distances[i][j]
			}

			payload := make(map[string]any)
			if j < len(queryResp.Metadatas[i]) {
				payload = queryResp.Metadatas[i][j]
			}
			if j < len(queryResp.Documents[i]) && queryResp.Documents[i][j] != "" {
				payload["content"] = queryResp.Documents[i][j]
			}

			results = append(results, SearchResult{
				ID:      id,
				Score:   score,
				Payload: payload,
			})
		}
	}

	return results, nil
}

func (s *ChromaDBStore) Delete(ctx context.Context, ids []string) error {
	body := chromaDeleteRequest{IDs: ids}
	resp, err := s.doRequest(ctx, http.MethodPost,
		"/api/v1/collections/"+s.collection+"/delete", body)
	if err != nil {
		if errors.Is(err, ErrCollectionNotFound) {
			return nil
		}
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *ChromaDBStore) RecreateCollection(ctx context.Context) error {
	resp, err := s.doRequest(ctx, http.MethodDelete,
		"/api/v1/collections/"+s.collection, nil)
	if err != nil {
		if !errors.Is(err, ErrCollectionNotFound) {
			return err
		}
	} else {
		resp.Body.Close()
	}

	body := chromaCreateCollectionReq{
		Name:     s.collection,
		Metadata: map[string]any{"hnsw:space": "cosine"},
	}

	resp, err = s.doRequest(ctx, http.MethodPost,
		"/api/v1/collections", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (s *ChromaDBStore) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
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
		if resp.StatusCode == http.StatusInternalServerError &&
			strings.Contains(string(bodyBytes), "does not exist") {
			return nil, ErrCollectionNotFound
		}
		return nil, fmt.Errorf("chromadb returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

type chromaAddRequest struct {
	IDs        []string         `json:"ids"`
	Embeddings [][]float32      `json:"embeddings"`
	Metadatas  []map[string]any `json:"metadatas"`
	Documents  []string         `json:"documents,omitempty"`
}

type chromaQueryRequest struct {
	QueryEmbeddings [][]float32 `json:"query_embeddings"`
	NResults        int         `json:"n_results"`
	Include         []string    `json:"include,omitempty"`
}

type chromaQueryResponse struct {
	IDs       [][]string         `json:"ids"`
	Distances [][]float32        `json:"distances"`
	Metadatas [][]map[string]any `json:"metadatas"`
	Documents [][]string         `json:"documents"`
}

type chromaDeleteRequest struct {
	IDs []string `json:"ids"`
}

type chromaCreateCollectionReq struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
