package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/saterdoe/oberth/internal/repoanalyzer"
)

func (s *Server) handleAnalyzeRepository(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(r.URL.Query().Get("path"))
	if root == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "path is required", nil)
		return
	}
	maxFiles, _ := strconv.Atoi(r.URL.Query().Get("max_files"))
	result, err := repoanalyzer.Analyze(root, repoanalyzer.Options{MaxFiles: maxFiles})
	if err != nil {
		respondError(w, http.StatusBadRequest, "ANALYZE_ERROR", err.Error(), nil)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleSearchRepository(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(r.URL.Query().Get("path"))
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if root == "" || query == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "path and query are required", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	matches, err := repoanalyzer.Search(root, query, repoanalyzer.SearchOptions{Limit: limit})
	if err != nil {
		respondError(w, http.StatusBadRequest, "SEARCH_ERROR", err.Error(), nil)
		return
	}
	symbols, err := repoanalyzer.SearchSymbols(root, query, limit)
	if err != nil {
		respondError(w, http.StatusBadRequest, "SEARCH_ERROR", err.Error(), nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"matches": matches, "symbols": symbols})
}
