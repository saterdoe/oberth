package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleAnalyzeRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/repo/analyze?path="+url.QueryEscape(root), nil)
	rec := httptest.NewRecorder()
	(&Server{}).handleAnalyzeRepository(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			Metadata struct {
				PrimaryLanguage string `json:"primary_language"`
			} `json:"metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Metadata.PrimaryLanguage != "Go" {
		t.Fatalf("language=%q", body.Data.Metadata.PrimaryLanguage)
	}
}

func TestHandleSearchRepositoryRequiresQuery(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/repo/search?path="+url.QueryEscape(t.TempDir()), nil)
	rec := httptest.NewRecorder()
	(&Server{}).handleSearchRepository(rec, req)
	if rec.Code != 400 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
