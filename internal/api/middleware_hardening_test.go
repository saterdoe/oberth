package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestHardeningRequiresJSONAndLimitsBodies(t *testing.T) {
	h := RequestHardening(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := r.Body.Read(make([]byte, maxRequestBodyBytes+1))
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader("x"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(make([]byte, maxRequestBodyBytes+1)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestLocalRateLimitRejectsBurst(t *testing.T) {
	h := LocalRateLimit(1, time.Minute, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for want, code := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		h.ServeHTTP(rec, req)
		if rec.Code != code {
			t.Fatalf("request %d: want %d got %d", want, code, rec.Code)
		}
	}
}

func FuzzRequestHardeningMalformed(f *testing.F) {
	f.Add("application/json", "{")
	f.Add("text/plain", "hello")
	f.Fuzz(func(t *testing.T, contentType, body string) {
		if len(body) > 2*maxRequestBodyBytes {
			t.Skip()
		}
		h := RequestHardening(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code < 100 || rec.Code > 599 {
			t.Fatalf("invalid status %d", rec.Code)
		}
	})
}
