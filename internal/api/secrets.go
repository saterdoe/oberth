package api

import (
	"encoding/json"
	"net/http"

	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

func (s *Server) handleScanSecrets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	result := secretspkg.Scan(req.Content)
	respondJSON(w, http.StatusOK, result)
}
