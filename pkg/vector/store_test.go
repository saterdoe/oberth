package vector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saterdoe/oberth/pkg/vector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterfaceContract(t *testing.T) {
	var _ vector.VectorStore = (*vector.QdrantStore)(nil)
}

type reqCapture struct {
	Method string
	Path   string
	Body   map[string]any
}

func TestQdrantStore_Ping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/healthz", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL(server.URL))
	require.NoError(t, store.Ping(context.Background()))
}

func TestChromaDBStore_Ping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/heartbeat", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := vector.NewChromaDBStore("test-col", 3, vector.WithChromaBaseURL(server.URL))
	require.NoError(t, store.Ping(context.Background()))
}

func TestQdrantStore_Upsert(t *testing.T) {
	reqCh := make(chan reqCapture, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		reqCh <- reqCapture{Method: r.Method, Path: r.URL.String(), Body: body}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":{"operation_id":1,"status":"completed"},"status":"ok","time":0.001}`))
	}))
	defer server.Close()

	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL(server.URL))
	err := store.Upsert(context.Background(), []vector.Point{
		{ID: "id-1", Vector: []float32{0.1, 0.2, 0.3}, Payload: map[string]any{"key": "val"}},
	})
	require.NoError(t, err)

	req := <-reqCh
	assert.Equal(t, http.MethodPut, req.Method)
	assert.Equal(t, "/collections/test-col/points?wait=true", req.Path)
	points, ok := req.Body["points"].([]any)
	require.True(t, ok)
	require.Len(t, points, 1)
	pt := points[0].(map[string]any)
	assert.Equal(t, "eb66c623-572f-5a2a-9167-ce017dee0541", pt["id"])
}

func TestQdrantStore_Search(t *testing.T) {
	reqCh := make(chan reqCapture, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		reqCh <- reqCapture{Method: r.Method, Path: r.URL.String(), Body: body}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":[{"id":"id-1","score":0.95,"payload":{"key":"val"}}],"status":"ok","time":0.001}`))
	}))
	defer server.Close()

	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL(server.URL))
	results, err := store.Search(context.Background(), []float32{0.1, 0.2, 0.3}, 5)
	require.NoError(t, err)

	req := <-reqCh
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "/collections/test-col/points/search", req.Path)
	assert.Equal(t, float64(5), req.Body["limit"])
	assert.Equal(t, true, req.Body["with_payload"])

	require.Len(t, results, 1)
	assert.Equal(t, "id-1", results[0].ID)
	assert.Equal(t, float32(0.95), results[0].Score)
	assert.Equal(t, "val", results[0].Payload["key"])
}

func TestQdrantStore_Delete(t *testing.T) {
	reqCh := make(chan reqCapture, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		reqCh <- reqCapture{Method: r.Method, Path: r.URL.String(), Body: body}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":{"operation_id":2,"status":"completed"},"status":"ok","time":0.001}`))
	}))
	defer server.Close()

	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL(server.URL))
	err := store.Delete(context.Background(), []string{"id-1", "id-2"})
	require.NoError(t, err)

	req := <-reqCh
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "/collections/test-col/points/delete", req.Path)
	points, ok := req.Body["points"].([]any)
	require.True(t, ok)
	assert.Equal(t, "eb66c623-572f-5a2a-9167-ce017dee0541", points[0])
	assert.Equal(t, "d5d5b1c8-0e94-5e23-80db-7bc056e06497", points[1])
}

func TestQdrantStore_RecreateCollection(t *testing.T) {
	reqCh := make(chan reqCapture, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&body)
		}
		reqCh <- reqCapture{Method: r.Method, Path: r.URL.String(), Body: body}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":true,"status":"ok","time":0.001}`))
	}))
	defer server.Close()

	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL(server.URL))
	err := store.RecreateCollection(context.Background())
	require.NoError(t, err)

	close(reqCh)
	var reqs []reqCapture
	for r := range reqCh {
		reqs = append(reqs, r)
	}

	require.Len(t, reqs, 2)
	assert.Equal(t, http.MethodDelete, reqs[0].Method)
	assert.Equal(t, "/collections/test-col", reqs[0].Path)
	assert.Equal(t, http.MethodPut, reqs[1].Method)
	assert.Equal(t, "/collections/test-col", reqs[1].Path)

	vectors, ok := reqs[1].Body["vectors"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(3), vectors["size"])
	assert.Equal(t, "Cosine", vectors["distance"])
}

func TestQdrantStore_CollectionNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"status":{"error":"Not found"},"time":0.001}`))
	}))
	defer server.Close()

	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL(server.URL))
	_, err := store.Search(context.Background(), []float32{0.1, 0.2, 0.3}, 5)
	require.Error(t, err)
	assert.ErrorIs(t, err, vector.ErrCollectionNotFound)
}

func TestQdrantStore_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":{"error":"Internal error"},"time":0.001}`))
	}))
	defer server.Close()

	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL(server.URL))
	err := store.Upsert(context.Background(), []vector.Point{
		{ID: "id-1", Vector: []float32{0.1, 0.2, 0.3}},
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, vector.ErrCollectionNotFound)
}

func TestQdrantStore_ConnectionFailed(t *testing.T) {
	store := vector.NewQdrantStore("test-col", 3, vector.WithBaseURL("http://127.0.0.1:1"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := store.Upsert(ctx, []vector.Point{
		{ID: "id-1", Vector: []float32{0.1, 0.2, 0.3}},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, vector.ErrConnectionFailed)
}
