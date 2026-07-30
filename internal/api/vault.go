package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/saterdoe/oberth/internal/vault"
)

type VaultStatusResponse struct {
	NoteCount   int    `json:"note_count"`
	VaultRoot   string `json:"vault_root"`
	LastIndexed string `json:"last_indexed,omitempty"`
}

func (s *Server) handleGetVaultStatus(w http.ResponseWriter, r *http.Request) {
	notes, err := s.vaultConn.ListAllNotes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list vault notes", nil)
		return
	}

	respondJSON(w, http.StatusOK, VaultStatusResponse{
		NoteCount: len(notes),
		VaultRoot: s.vaultConn.Root(),
	})
}

func (s *Server) handleListVaultNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := s.vaultConn.ListAllNotes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list vault notes", nil)
		return
	}
	if notes == nil {
		notes = []vault.Note{}
	}
	respondJSON(w, http.StatusOK, notes)
}

func (s *Server) handleSearchVault(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "q is required", nil)
		return
	}
	limit := parseIntParam(r.URL.Query().Get("limit"), 20)
	if limit <= 0 {
		limit = 20
	}
	taskType := strings.TrimSpace(r.URL.Query().Get("task_type"))

	if s.searcher != nil {
		results, metrics, err := s.searcher.SearchWithMetrics(r.Context(), query, limit, taskType)
		if err == nil {
			respondJSON(w, http.StatusOK, map[string]any{"results": results, "metrics": metrics})
			return
		}
	}

	notes, err := s.vaultConn.ListAllNotes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to search vault notes", nil)
		return
	}
	lowerQuery := strings.ToLower(query)
	type fallbackResult struct {
		Note  vault.Note `json:"note"`
		Score float64    `json:"score"`
	}
	results := make([]fallbackResult, 0)
	for _, note := range notes {
		path := strings.ToLower(note.Path)
		content := strings.ToLower(note.Content)
		score := 0.0
		if strings.Contains(path, lowerQuery) {
			score += 2
		}
		if strings.Contains(content, lowerQuery) {
			score += 1
		}
		if score > 0 {
			results = append(results, fallbackResult{Note: note, Score: score})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}

	respondJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) handleReindexVault(w http.ResponseWriter, r *http.Request) {
	if s.indexer == nil {
		respondError(w, http.StatusServiceUnavailable, "SEMANTIC_SEARCH_DISABLED", "vault indexer is unavailable", nil)
		return
	}
	result, err := s.indexer.Reindex(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to reindex vault", nil)
		return
	}

	respondJSON(w, http.StatusOK, result)
}
