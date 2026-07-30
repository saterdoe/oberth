package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/internal/providersecret"
	"github.com/saterdoe/oberth/pkg/llm"
)

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

// CreateProviderRequest is the JSON body for creating a new provider.
type CreateProviderRequest struct {
	Name         string  `json:"name"`
	ProviderType string  `json:"provider_type"`
	BaseURL      *string `json:"base_url,omitempty"`
	APIKey       *string `json:"api_key,omitempty"`
	DefaultModel string  `json:"default_model"`
	Models       string  `json:"models"`
	IsActive     *bool   `json:"is_active,omitempty"`
	Priority     *int    `json:"priority,omitempty"`
	RateLimitRPM *int    `json:"rate_limit_rpm,omitempty"`
}

// UpdateProviderRequest is the JSON body for partially updating a provider.
// Only non-nil fields are applied.
type UpdateProviderRequest struct {
	Name         *string `json:"name,omitempty"`
	ProviderType *string `json:"provider_type,omitempty"`
	BaseURL      *string `json:"base_url,omitempty"`
	APIKey       *string `json:"api_key,omitempty"`
	DefaultModel *string `json:"default_model,omitempty"`
	Models       *string `json:"models,omitempty"`
	IsActive     *bool   `json:"is_active,omitempty"`
	Priority     *int    `json:"priority,omitempty"`
	RateLimitRPM *int    `json:"rate_limit_rpm,omitempty"`
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func respondError(w http.ResponseWriter, status int, code string, message string, details any) {
	body := map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	if details != nil {
		body["error"].(map[string]any)["details"] = details
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// GET /api/v1/providers
func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.providers.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list providers", nil)
		return
	}
	if providers == nil {
		providers = []repos.Provider{}
	}
	respondJSON(w, http.StatusOK, providers)
}

// POST /api/v1/providers
func (s *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req CreateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if req.ProviderType == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "provider_type is required", nil)
		return
	}
	if req.DefaultModel == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "default_model is required", nil)
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}

	encryptedKey, err := s.sealProviderAPIKey(req.APIKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PROVIDER_SECRET_UNAVAILABLE", err.Error(), nil)
		return
	}

	p := &repos.Provider{
		Name:            req.Name,
		ProviderType:    req.ProviderType,
		BaseURL:         req.BaseURL,
		APIKeyEncrypted: encryptedKey,
		DefaultModel:    req.DefaultModel,
		Models:          req.Models,
		IsActive:        isActive,
		Priority:        priority,
		RateLimitRPM:    req.RateLimitRPM,
	}

	if err := s.providers.Create(r.Context(), p); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create provider", nil)
		return
	}

	respondJSON(w, http.StatusCreated, p)
}

// GET /api/v1/providers/{id}
func (s *Server) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider ID", nil)
		return
	}

	p, err := s.providers.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "provider not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get provider", nil)
		return
	}

	respondJSON(w, http.StatusOK, p)
}

// PUT /api/v1/providers/{id}
func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider ID", nil)
		return
	}

	var req UpdateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	existing, err := s.providers.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "provider not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch provider", nil)
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.ProviderType != nil {
		existing.ProviderType = *req.ProviderType
	}
	if req.BaseURL != nil {
		existing.BaseURL = req.BaseURL
	}
	if req.APIKey != nil {
		encryptedKey, sealErr := s.sealProviderAPIKey(req.APIKey)
		if sealErr != nil {
			respondError(w, http.StatusInternalServerError, "PROVIDER_SECRET_UNAVAILABLE", sealErr.Error(), nil)
			return
		}
		existing.APIKeyEncrypted = encryptedKey
	}
	if req.DefaultModel != nil {
		existing.DefaultModel = *req.DefaultModel
	}
	if req.Models != nil {
		existing.Models = *req.Models
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}
	if req.Priority != nil {
		existing.Priority = *req.Priority
	}
	if req.RateLimitRPM != nil {
		existing.RateLimitRPM = req.RateLimitRPM
	}

	if err := s.providers.Update(r.Context(), existing); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update provider", nil)
		return
	}

	respondJSON(w, http.StatusOK, existing)
}

// DELETE /api/v1/providers/{id}
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider ID", nil)
		return
	}

	if err := s.providers.Delete(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "provider not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete provider", nil)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/providers/{id}/fetch-models
// Calls the provider's /v1/models endpoint (OpenAI-compatible) and returns available model IDs.
func (s *Server) handleFetchProviderModels(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider ID", nil)
		return
	}

	p, err := s.providers.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "provider not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get provider", nil)
		return
	}

	baseURL := ""
	if p.BaseURL != nil {
		baseURL = strings.TrimRight(*p.BaseURL, "/")
	}
	if baseURL == "" {
		respondError(w, http.StatusBadRequest, "NO_BASE_URL", "provider has no base URL", nil)
		return
	}

	// Build the models endpoint URL
	modelsURL := baseURL
	if !strings.HasSuffix(modelsURL, "/v1") {
		modelsURL += "/v1"
	}
	modelsURL += "/models"

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), "GET", modelsURL, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "REQUEST_ERROR", "failed to create request", nil)
		return
	}

	if p.APIKeyEncrypted != nil && *p.APIKeyEncrypted != "" {
		apiKey, openErr := providersecret.Open(s.cfg.Auth.Token, *p.APIKeyEncrypted)
		if openErr != nil {
			respondError(w, http.StatusInternalServerError, "PROVIDER_SECRET_UNAVAILABLE", "provider credential could not be decrypted", nil)
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		respondError(w, http.StatusBadGateway, "FETCH_ERROR", fmt.Sprintf("failed to fetch models: %v", err), nil)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusBadGateway, "READ_ERROR", "failed to read response", nil)
		return
	}

	if resp.StatusCode != http.StatusOK {
		respondError(w, http.StatusBadGateway, "API_ERROR", fmt.Sprintf("provider returned status %d: %s", resp.StatusCode, string(body)), nil)
		return
	}

	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		respondError(w, http.StatusBadGateway, "PARSE_ERROR", "failed to parse models response", nil)
		return
	}

	var modelIDs []string
	for _, m := range modelsResp.Data {
		if m.ID != "" {
			modelIDs = append(modelIDs, m.ID)
		}
	}
	if len(modelIDs) > 0 {
		p.Models = strings.Join(modelIDs, ",")
		if strings.TrimSpace(p.DefaultModel) == "" {
			p.DefaultModel = modelIDs[0]
		}
		if err := s.providers.Update(r.Context(), p); err != nil {
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to save discovered models", nil)
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"models": modelIDs,
		"source": "api",
	})
}

func (s *Server) sealProviderAPIKey(apiKey *string) (*string, error) {
	if apiKey == nil {
		return nil, nil
	}
	sealed, err := providersecret.Seal(s.cfg.Auth.Token, *apiKey)
	if err != nil {
		return nil, err
	}
	return &sealed, nil
}

// POST /api/v1/providers/{id}/test
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid provider ID", nil)
		return
	}

	p, err := s.providers.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "provider not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get provider", nil)
		return
	}

	if p.APIKeyEncrypted != nil && *p.APIKeyEncrypted != "" {
		apiKey, openErr := providersecret.Open(s.cfg.Auth.Token, *p.APIKeyEncrypted)
		if openErr != nil {
			respondError(w, http.StatusInternalServerError, "PROVIDER_SECRET_UNAVAILABLE", "provider credential could not be decrypted", nil)
			return
		}
		p.APIKeyEncrypted = &apiKey
	}
	provider, err := gateway.BuildProvider(*p)
	if err != nil {
		respondError(w, http.StatusBadRequest, "PROVIDER_INVALID", err.Error(), nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	result, err := provider.Chat(ctx, providerVerificationRequest(p.DefaultModel))
	if err != nil {
		respondError(w, http.StatusBadGateway, "PROVIDER_UNAVAILABLE", fmt.Sprintf("provider verification failed: %v", err), nil)
		return
	}
	if result == nil || strings.TrimSpace(result.Content) == "" {
		respondError(w, http.StatusBadGateway, "PROVIDER_INVALID_RESPONSE", "provider returned an empty completion", nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"message":       p.Name + " completion verified",
		"model":         result.Model,
		"input_tokens":  result.InputTokens,
		"output_tokens": result.OutputTokens,
	})
}

func providerVerificationRequest(model string) llm.ChatRequest {
	return llm.ChatRequest{
		Model:       model,
		Messages:    []llm.Message{{Role: "user", Content: "Reply with exactly: OK"}},
		Temperature: 0,
		// Reasoning-capable local models may spend part of the output budget on
		// hidden thinking tokens before producing the short visible response.
		MaxTokens: 128,
	}
}
