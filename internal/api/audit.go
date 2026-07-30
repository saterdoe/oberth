package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/db/repos"
)

func (s *Server) handleListAuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := repos.AuditFilter{
		Offset: parseIntParam(q.Get("offset"), 0),
		Limit:  parseIntParam(q.Get("limit"), 100),
	}

	if action := q.Get("action"); action != "" {
		filter.Action = &action
	}
	if actor := q.Get("actor"); actor != "" {
		filter.Actor = &actor
	}
	if since := q.Get("since"); since != "" {
		parsed, err := parseAuditTime(since, false)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_DATE", "invalid since date", nil)
			return
		}
		filter.Since = &parsed
	}
	if until := q.Get("until"); until != "" {
		parsed, err := parseAuditTime(until, true)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_DATE", "invalid until date", nil)
			return
		}
		filter.Until = &parsed
	}

	entries, err := s.audit.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list audit log", nil)
		return
	}
	if entries == nil {
		entries = []repos.AuditLogEntry{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   len(entries),
	})
}

func parseAuditTime(value string, endOfDay bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		return parsed.Add(24 * time.Hour), nil
	}
	return parsed, nil
}

type logTerminalCommandRequest struct {
	SessionID  string `json:"session_id"`
	Command    string `json:"command"`
	Args       string `json:"args,omitempty"`
	WorkingDir string `json:"working_dir,omitempty"`
	Approved   bool   `json:"approved"`
	Proposed   bool   `json:"proposed"`
}

func (s *Server) handleLogTerminalCommand(w http.ResponseWriter, r *http.Request) {
	var req logTerminalCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", nil)
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		respondError(w, http.StatusBadRequest, "MISSING_COMMAND", "command is required", nil)
		return
	}
	var sessionID *uuid.UUID
	if req.SessionID != "" {
		if parsed, err := uuid.Parse(req.SessionID); err == nil {
			sessionID = &parsed
		}
	}
	action := "terminal_command_approved"
	if req.Proposed && !req.Approved {
		action = "terminal_command_rejected"
	}
	details := map[string]any{
		"command":  req.Command,
		"approved": req.Approved,
		"proposed": req.Proposed,
	}
	if req.Args != "" {
		details["args"] = req.Args
	}
	if req.WorkingDir != "" {
		details["working_dir"] = req.WorkingDir
	}
	s.logAudit(r.Context(), sessionID, action, "user:local", details)
	respondJSON(w, http.StatusOK, map[string]string{"status": "logged"})
}
