package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
)

type ListSessionsResponse struct {
	Sessions []repos.Session `json:"sessions"`
	Total    int             `json:"total"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	filter := repos.SessionFilter{
		Offset: parseIntParam(r.URL.Query().Get("offset"), 0),
		Limit:  parseIntParam(r.URL.Query().Get("limit"), 100),
	}
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = &status
	}
	sessions, err := s.sessions.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list sessions", nil)
		return
	}
	if sessions == nil {
		sessions = []repos.Session{}
	}
	respondJSON(w, http.StatusOK, ListSessionsResponse{Sessions: sessions, Total: len(sessions)})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid session ID", nil)
		return
	}
	session, err := s.sessions.GetByID(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "session not found", nil)
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get session", nil)
		return
	}
	respondJSON(w, http.StatusOK, session)
}
