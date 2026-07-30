package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
)

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

// CreateRoutingRuleRequest is the JSON body for creating a new routing rule.
type CreateRoutingRuleRequest struct {
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Priority         int            `json:"priority"`
	IsActive         *bool          `json:"is_active,omitempty"`
	MatchRepoPattern *string        `json:"match_repo_pattern,omitempty"`
	MatchTaskType    *string        `json:"match_task_type,omitempty"`
	MatchUserID      *string        `json:"match_user_id,omitempty"`
	ProviderID       string         `json:"provider_id"`
	Model            string         `json:"model"`
	ExecutionGraph   map[string]any `json:"execution_graph,omitempty"`
}

// UpdateRoutingRuleRequest is the JSON body for partially updating a routing rule.
type UpdateRoutingRuleRequest struct {
	Name             *string        `json:"name,omitempty"`
	Description      *string        `json:"description,omitempty"`
	Priority         *int           `json:"priority,omitempty"`
	IsActive         *bool          `json:"is_active,omitempty"`
	MatchRepoPattern *string        `json:"match_repo_pattern,omitempty"`
	MatchTaskType    *string        `json:"match_task_type,omitempty"`
	MatchUserID      *string        `json:"match_user_id,omitempty"`
	ProviderID       *string        `json:"provider_id,omitempty"`
	Model            *string        `json:"model,omitempty"`
	ExecutionGraph   map[string]any `json:"execution_graph,omitempty"`
}

// ReorderRequest is the JSON body for reordering routing rules.
type ReorderRequest struct {
	RuleIDs []string `json:"rule_ids"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// GET /api/v1/routing-rules
func (s *Server) handleListRoutingRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.routingRules.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list routing rules", nil)
		return
	}
	if rules == nil {
		rules = []repos.RoutingRule{}
	}
	respondJSON(w, http.StatusOK, rules)
}

// POST /api/v1/routing-rules
func (s *Server) handleCreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	var req CreateRoutingRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if req.ProviderID == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "provider_id is required", nil)
		return
	}
	if req.Model == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "model is required", nil)
		return
	}

	providerID, err := uuid.Parse(req.ProviderID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider_id", nil)
		return
	}
	if _, err := s.providers.GetByID(r.Context(), providerID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "provider_id does not reference an existing provider", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to validate provider_id", nil)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var matchUserID *uuid.UUID
	if req.MatchUserID != nil {
		parsed, err := uuid.Parse(*req.MatchUserID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid match_user_id", nil)
			return
		}
		matchUserID = &parsed
	}

	var execGraph json.RawMessage
	if req.ExecutionGraph != nil {
		data, err := json.Marshal(req.ExecutionGraph)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid execution_graph", nil)
			return
		}
		execGraph = data
	}

	rule := &repos.RoutingRule{
		Name:             req.Name,
		Description:      req.Description,
		Priority:         req.Priority,
		IsActive:         isActive,
		MatchRepoPattern: req.MatchRepoPattern,
		MatchTaskType:    req.MatchTaskType,
		MatchUserID:      matchUserID,
		ProviderID:       providerID,
		Model:            req.Model,
		ExecutionGraph:   execGraph,
	}

	if err := s.routingRules.Create(r.Context(), rule); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create routing rule", nil)
		return
	}

	respondJSON(w, http.StatusCreated, rule)
}

// GET /api/v1/routing-rules/{id}
func (s *Server) handleGetRoutingRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid routing rule ID", nil)
		return
	}

	rule, err := s.routingRules.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "routing rule not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get routing rule", nil)
		return
	}

	respondJSON(w, http.StatusOK, rule)
}

// PUT /api/v1/routing-rules/{id}
func (s *Server) handleUpdateRoutingRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid routing rule ID", nil)
		return
	}

	var req UpdateRoutingRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	existing, err := s.routingRules.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "routing rule not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch routing rule", nil)
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}
	if req.MatchRepoPattern != nil {
		existing.MatchRepoPattern = req.MatchRepoPattern
	}
	if req.MatchTaskType != nil {
		existing.MatchTaskType = req.MatchTaskType
	}
	if req.MatchUserID != nil {
		parsed, parseErr := uuid.Parse(*req.MatchUserID)
		if parseErr != nil {
			respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid match_user_id", nil)
			return
		}
		existing.MatchUserID = &parsed
	}
	if req.ProviderID != nil {
		parsed, parseErr := uuid.Parse(*req.ProviderID)
		if parseErr != nil {
			respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider_id", nil)
			return
		}
		if _, providerErr := s.providers.GetByID(r.Context(), parsed); providerErr != nil {
			if errors.Is(providerErr, db.ErrNotFound) {
				respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "provider_id does not reference an existing provider", nil)
				return
			}
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to validate provider_id", nil)
			return
		}
		existing.ProviderID = parsed
	}
	if req.Model != nil {
		existing.Model = *req.Model
	}
	if req.ExecutionGraph != nil {
		data, marshalErr := json.Marshal(req.ExecutionGraph)
		if marshalErr != nil {
			respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid execution_graph", nil)
			return
		}
		existing.ExecutionGraph = data
	}

	if err := s.routingRules.Update(r.Context(), existing); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update routing rule", nil)
		return
	}

	respondJSON(w, http.StatusOK, existing)
}

// DELETE /api/v1/routing-rules/{id}
func (s *Server) handleDeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid routing rule ID", nil)
		return
	}

	if err := s.routingRules.Delete(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "routing rule not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete routing rule", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/routing-rules/reorder
func (s *Server) handleReorderRoutingRules(w http.ResponseWriter, r *http.Request) {
	var req ReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if len(req.RuleIDs) == 0 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "rule_ids is required", nil)
		return
	}

	ids := make([]uuid.UUID, len(req.RuleIDs))
	for i, idStr := range req.RuleIDs {
		parsed, err := uuid.Parse(idStr)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid rule ID in rule_ids", nil)
			return
		}
		ids[i] = parsed
	}

	if err := s.routingRules.Reorder(r.Context(), ids); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to reorder routing rules", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
