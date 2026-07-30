package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
)

type CreateBudgetRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	ProviderID  *string    `json:"provider_id,omitempty"`
	SoftLimit   float64    `json:"soft_limit"`
	HardLimit   float64    `json:"hard_limit"`
	Period      string     `json:"period"`
	PeriodStart *time.Time `json:"period_start,omitempty"`
	IsActive    *bool      `json:"is_active,omitempty"`
}

type UpdateBudgetRequest struct {
	Name        *string    `json:"name,omitempty"`
	Description *string    `json:"description,omitempty"`
	ProviderID  *string    `json:"provider_id,omitempty"`
	SoftLimit   *float64   `json:"soft_limit,omitempty"`
	HardLimit   *float64   `json:"hard_limit,omitempty"`
	Period      *string    `json:"period,omitempty"`
	PeriodStart *time.Time `json:"period_start,omitempty"`
	IsActive    *bool      `json:"is_active,omitempty"`
}

func (s *Server) handleListBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := s.budgets.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list budgets", nil)
		return
	}
	if budgets == nil {
		budgets = []repos.Budget{}
	}
	respondJSON(w, http.StatusOK, budgets)
}

func (s *Server) handleGetBudget(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid budget ID", nil)
		return
	}

	budget, err := s.budgets.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "budget not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get budget", nil)
		return
	}

	respondJSON(w, http.StatusOK, budget)
}

func (s *Server) handleCreateBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if req.SoftLimit <= 0 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_limit must be positive", nil)
		return
	}
	if req.HardLimit <= 0 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "hard_limit must be positive", nil)
		return
	}
	if req.Period == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "period is required", nil)
		return
	}

	var providerID *uuid.UUID
	if req.ProviderID != nil {
		parsed, err := uuid.Parse(*req.ProviderID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider_id", nil)
			return
		}
		providerID = &parsed
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	periodStart := time.Now()
	if req.PeriodStart != nil {
		periodStart = *req.PeriodStart
	}

	budget := &repos.Budget{
		Name:        req.Name,
		Description: req.Description,
		ProviderID:  providerID,
		SoftLimit:   req.SoftLimit,
		HardLimit:   req.HardLimit,
		Period:      req.Period,
		PeriodStart: periodStart,
		IsActive:    isActive,
	}

	if err := s.budgets.Create(r.Context(), budget); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create budget", nil)
		return
	}

	respondJSON(w, http.StatusCreated, budget)
}

func (s *Server) handleUpdateBudget(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid budget ID", nil)
		return
	}

	var req UpdateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	existing, err := s.budgets.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "budget not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch budget", nil)
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.ProviderID != nil {
		parsed, parseErr := uuid.Parse(*req.ProviderID)
		if parseErr != nil {
			respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider_id", nil)
			return
		}
		existing.ProviderID = &parsed
	}
	if req.SoftLimit != nil {
		existing.SoftLimit = *req.SoftLimit
	}
	if req.HardLimit != nil {
		existing.HardLimit = *req.HardLimit
	}
	if req.Period != nil {
		existing.Period = *req.Period
	}
	if req.PeriodStart != nil {
		existing.PeriodStart = *req.PeriodStart
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := s.budgets.Update(r.Context(), existing); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update budget", nil)
		return
	}

	respondJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteBudget(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid budget ID", nil)
		return
	}

	if err := s.budgets.Delete(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "budget not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete budget", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
