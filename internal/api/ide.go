package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/idelaunch"
)

func (s *Server) handleListLaunchers(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, idelaunch.Available())
}

func (s *Server) handleOpenRunInIDE(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid run ID", nil)
		return
	}
	var req struct {
		IDE  string `json:"ide"`
		File string `json:"file"`
		Line int    `json:"line"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	var worktree string
	if err := s.pool.QueryRow(r.Context(), `SELECT worktree_path FROM task_runs WHERE id=$1`, id).Scan(&worktree); err != nil {
		respondError(w, http.StatusNotFound, "RUN_NOT_FOUND", "run not found", nil)
		return
	}
	if err := idelaunch.Open(req.IDE, worktree, req.File, req.Line); err != nil {
		respondError(w, http.StatusBadGateway, "IDE_LAUNCH_FAILED", err.Error(), nil)
		return
	}
	_ = s.appendRunEvent(r.Context(), id, "ide_opened", map[string]any{"ide": req.IDE, "file": req.File, "line": req.Line})
	respondJSON(w, http.StatusOK, map[string]any{"status": "opened", "ide": req.IDE, "worktree": worktree, "file": req.File, "line": req.Line})
}
