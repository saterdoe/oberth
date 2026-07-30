package api

import (
	"net/http"

	"github.com/saterdoe/oberth/internal/localprovider"
)

func (s *Server) handleDiscoverLocalProviders(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, localprovider.Discover(r.Context()))
}
