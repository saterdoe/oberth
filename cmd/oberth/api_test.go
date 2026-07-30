package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApiURL(t *testing.T) {
	apiPort = 9090
	assert.Equal(t, "http://localhost:9090/api/v1/health", apiURL("/health"))
	assert.Equal(t, "http://localhost:9090/api/v1/sessions?limit=10", apiURL("/sessions?limit=10"))
}

func TestApiRequest_GET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/test", r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()
	apiPort = portFromURL(t, ts.URL)

	data, err := apiGET("/test")
	require.NoError(t, err)
	assert.Contains(t, string(data), `"status":"ok"`)
}

func TestApiRequest_POST(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/create", r.URL.String())
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"data":{"id":"abc"}}`))
	}))
	defer ts.Close()
	apiPort = portFromURL(t, ts.URL)

	data, err := apiPOST("/create", `{"name":"test"}`)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"abc"`)
}

func TestApiRequest_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":"INVALID_TRANSITION","message":"run is not awaiting an outcome"}}`))
	}))
	defer ts.Close()
	apiPort = portFromURL(t, ts.URL)

	_, err := apiGET("/fail")
	assert.Error(t, err)
	apiErr, ok := asAPIError(err)
	require.True(t, ok)
	assert.Equal(t, 400, apiErr.Status)
	assert.Equal(t, "INVALID_TRANSITION", apiErr.Code)
	assert.Contains(t, err.Error(), "oberth status")
}

func TestApiUnwrapGET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"name":"test","count":42}}`))
	}))
	defer ts.Close()
	apiPort = portFromURL(t, ts.URL)

	var result struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	err := apiUnwrapGET("/unwrap", &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Count)
}

func TestApiUnwrapPOST(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"id":"xyz","status":"created"}}`))
	}))
	defer ts.Close()
	apiPort = portFromURL(t, ts.URL)

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	err := apiUnwrapPOST("/create", `{}`, &result)
	require.NoError(t, err)
	assert.Equal(t, "xyz", result.ID)
	assert.Equal(t, "created", result.Status)
}

func TestApiRequest_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer ts.Close()
	apiPort = portFromURL(t, ts.URL)

	var dest struct{}
	err := apiUnwrapGET("/badjson", &dest)
	assert.Error(t, err)
}

func TestApiRequest_NoDataField(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"other":1}`))
	}))
	defer ts.Close()
	apiPort = portFromURL(t, ts.URL)

	var dest struct{}
	err := apiUnwrapGET("/nodata", &dest)
	assert.Error(t, err)
}

func portFromURL(t *testing.T, rawURL string) int {
	t.Helper()
	parts := strings.Split(rawURL, ":")
	last := parts[len(parts)-1]
	p, err := strconv.Atoi(last)
	require.NoError(t, err)
	return p
}
