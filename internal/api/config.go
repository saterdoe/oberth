package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfg
	if cfg == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"message": "no config loaded, using defaults",
			"server": map[string]any{
				"host": "0.0.0.0",
				"port": 9090,
			},
		})
		return
	}
	respondJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "config updated (runtime only)"})
}

func (s *Server) handleValidateConfig(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"valid":  true,
		"errors": []string{},
	})
}
