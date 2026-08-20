package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/saterdoe/oberth/internal/codeindex"
)

func (s *Server) projectCodeIndex(r *http.Request) (*codeindex.Index, string, error) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return nil, "", err
	}
	var root string
	if err = s.pool.QueryRow(r.Context(), `SELECT path FROM projects WHERE id=$1`, id).Scan(&root); err != nil {
		return nil, "", err
	}
	var embedder codeindex.Embedder
	if s.searcher != nil {
		embedder = s.searcher.Embedder()
	}
	o := codeindex.Options{MaxFileBytes: s.cfg.CodeIndex.MaxFileBytes, MaxFiles: s.cfg.CodeIndex.MaxFiles, MaxChunks: s.cfg.CodeIndex.MaxChunks, MaxChunkLines: s.cfg.CodeIndex.MaxChunkLines, OverlapLines: s.cfg.CodeIndex.OverlapLines, Exclude: s.cfg.CodeIndex.Exclude}
	index, err := codeindex.OpenLocalWithIdentity(root, root, embedder, o)
	return index, root, err
}

func (s *Server) handleFindCodeMapNodes(w http.ResponseWriter, r *http.Request) {
	index, _, err := s.projectCodeIndex(r)
	if err != nil {
		s.respondCodeMapIndexError(w, err)
		return
	}
	result, err := index.FindGraphNodes(r.URL.Query().Get("query"), graphQueryLimit(r), r.URL.Query().Get("cursor"))
	if err != nil {
		respondCodeMapQueryError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleCodeMapNeighborhood(w http.ResponseWriter, r *http.Request) {
	index, _, err := s.projectCodeIndex(r)
	if err != nil {
		s.respondCodeMapIndexError(w, err)
		return
	}
	result, err := index.GraphNeighborhood(r.URL.Query().Get("node_id"), codeindex.GraphDirection(r.URL.Query().Get("direction")), graphQueryLimit(r), r.URL.Query().Get("cursor"))
	if err != nil {
		respondCodeMapQueryError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func graphQueryLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return codeindex.DefaultGraphNodeLimit
	}
	return min(limit, codeindex.MaxGraphNodeLimit)
}

func (s *Server) respondCodeMapIndexError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found", nil)
		return
	}
	respondError(w, http.StatusBadRequest, "CODE_MAP_UNAVAILABLE", "code map unavailable", nil)
}

func respondCodeMapQueryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, codeindex.ErrGraphNodeNotFound):
		respondError(w, http.StatusNotFound, "CODE_MAP_NODE_NOT_FOUND", "code map node not found", nil)
	case errors.Is(err, codeindex.ErrGraphCursorStale):
		respondError(w, http.StatusConflict, "CODE_MAP_STALE_CURSOR", "the code map changed; restart this query", nil)
	default:
		respondError(w, http.StatusBadRequest, "CODE_MAP_INVALID_QUERY", "invalid code map query", nil)
	}
}

func (s *Server) handleGetProjectCodeIndex(w http.ResponseWriter, r *http.Request) {
	index, _, err := s.projectCodeIndex(r)
	if err != nil {
		if err == pgx.ErrNoRows {
			respondError(w, 404, "PROJECT_NOT_FOUND", "project not found", nil)
		} else {
			respondError(w, 400, "CODE_INDEX_UNAVAILABLE", "code index unavailable", nil)
		}
		return
	}
	respondJSON(w, 200, index.Status())
}

func (s *Server) handleReindexProjectCode(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.CodeIndex.IsEnabled() {
		respondError(w, 409, "CODE_INDEX_DISABLED", "code indexing is disabled", nil)
		return
	}
	index, _, err := s.projectCodeIndex(r)
	if err != nil {
		if err == pgx.ErrNoRows {
			respondError(w, 404, "PROJECT_NOT_FOUND", "project not found", nil)
		} else {
			respondError(w, 400, "CODE_INDEX_UNAVAILABLE", "code index unavailable", nil)
		}
		return
	}
	metrics, err := index.Update(r.Context())
	if err != nil {
		respondError(w, 500, "CODE_REINDEX_FAILED", "repository code could not be indexed", nil)
		return
	}
	respondJSON(w, 200, map[string]any{"status": index.Status(), "metrics": metrics})
}
