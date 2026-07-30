package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/saterdoe/oberth/internal/cost"
	"github.com/saterdoe/oberth/internal/db/repos"
)

// RecordCallRequest is the JSON body for recording an LLM cost call.
type RecordCallRequest struct {
	SessionID    string  `json:"session_id"`
	ProviderID   string  `json:"provider_id"`
	Model        string  `json:"model"`
	TokensInput  int     `json:"tokens_input"`
	TokensOutput int     `json:"tokens_output"`
	CostInput    float64 `json:"cost_input"`
	CostOutput   float64 `json:"cost_output"`
	CacheHit     bool    `json:"cache_hit"`
}

func (s *Server) handleGetCostSummary(w http.ResponseWriter, r *http.Request) {
	sinceStr := r.URL.Query().Get("since")
	since := time.Now().Add(-30 * 24 * time.Hour)
	if sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_DATE", "invalid since (use RFC3339)", nil)
			return
		}
		since = t
	}

	summary, err := s.costLogs.GetSummary(r.Context(), since)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get cost summary", nil)
		return
	}

	respondJSON(w, http.StatusOK, summary)
}

func (s *Server) handleListCostLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := repos.CostLogFilter{
		Offset: parseIntParam(q.Get("offset"), 0),
		Limit:  parseIntParam(q.Get("limit"), 100),
	}

	if sinceStr := q.Get("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_DATE", "invalid since (use RFC3339)", nil)
			return
		}
		filter.Since = &t
	}

	logs, err := s.costLogs.List(r.Context(), filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list cost logs", nil)
		return
	}
	if logs == nil {
		logs = []repos.CostLog{}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"logs":  logs,
		"total": len(logs),
	})
}

func (s *Server) handleRecordCall(w http.ResponseWriter, r *http.Request) {
	var req RecordCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if req.SessionID == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "session_id is required", nil)
		return
	}
	if req.Model == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "model is required", nil)
		return
	}

	call := cost.CallRecord{
		SessionID:    req.SessionID,
		ProviderID:   req.ProviderID,
		Model:        req.Model,
		TokensInput:  req.TokensInput,
		TokensOutput: req.TokensOutput,
		CostInput:    req.CostInput,
		CostOutput:   req.CostOutput,
		CacheHit:     req.CacheHit,
	}

	alert, err := s.costTracker.RecordCall(r.Context(), call)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to record call", nil)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"alert": alert,
	})
}
