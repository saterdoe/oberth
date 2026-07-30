package api

import (
	"encoding/json"
	"net/http"

	"github.com/saterdoe/oberth/internal/cost"
)

func (s *Server) handleSimulateCosts(w http.ResponseWriter, r *http.Request) {
	var input cost.SimulationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if len(input.Scenarios) == 0 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "at least one scenario is required", nil)
		return
	}

	result, err := cost.Simulate(r.Context(), input, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "simulation failed", nil)
		return
	}

	respondJSON(w, http.StatusOK, result)
}
