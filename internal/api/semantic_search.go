package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/pkg/vector"
)

func (s *Server) handleGetSemanticSearch(w http.ResponseWriter, _ *http.Request) {
	if s.cfg == nil {
		respondError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "semantic search settings are unavailable", nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"enabled":    s.cfg.VectorDB.Engine != "disabled",
		"engine":     s.cfg.VectorDB.Engine,
		"embedder":   s.cfg.VectorDB.Embedder.Provider,
		"model":      s.cfg.VectorDB.Embedder.Model,
		"dimensions": s.cfg.VectorDB.Embedder.Dimensions,
		"migration":  "idle",
		"qdrant_url": s.cfg.VectorDB.Qdrant.URL,
		"collection": s.cfg.VectorDB.Qdrant.Collection,
	})
}

func (s *Server) handleMigrateSemanticSearch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Engine string `json:"engine"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	request.Engine = strings.ToLower(strings.TrimSpace(request.Engine))
	if request.Engine == "disabled" {
		if s.searcher != nil {
			s.searcher.SetVectorStore(nil)
		}
		s.cfg.VectorDB.Engine = "disabled"
		if err := config.SaveSemanticSettings(config.SemanticSettings{Engine: "disabled", QdrantCollection: s.cfg.VectorDB.Qdrant.Collection}); err != nil {
			respondError(w, http.StatusInternalServerError, "SETTINGS_WRITE_FAILED", err.Error(), nil)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"engine": "disabled", "status": "completed"})
		return
	}
	if s.indexer == nil {
		respondError(w, http.StatusServiceUnavailable, "INDEXER_UNAVAILABLE", "semantic indexer is unavailable", nil)
		return
	}
	dimensions := s.cfg.VectorDB.Embedder.Dimensions
	if dimensions <= 0 {
		dimensions = 384
	}
	var target vector.VectorStore
	activeCollection := s.cfg.VectorDB.Qdrant.Collection
	switch request.Engine {
	case "builtin", "local":
		path := s.cfg.VectorDB.Local.Path
		if path == "" {
			path = filepath.Join(filepath.Dir(s.cfg.Vault.Path), "vector", "index.json")
		}
		store, err := vector.NewLocalStore(path, dimensions)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "TARGET_UNAVAILABLE", err.Error(), nil)
			return
		}
		target = store
		request.Engine = "builtin"
	case "qdrant":
		baseCollection := qdrantBaseCollection(s.cfg.VectorDB.Qdrant.Collection)
		if baseCollection == "" {
			baseCollection = "oberth-notes"
		}
		activeCollection = fmt.Sprintf("%s-%s", baseCollection, time.Now().UTC().Format("20060102t150405"))
		target = vector.NewQdrantStore(
			activeCollection,
			dimensions,
			vector.WithBaseURL(s.cfg.VectorDB.Qdrant.URL),
		)
	default:
		respondError(w, http.StatusBadRequest, "UNSUPPORTED_ENGINE", "engine must be builtin, qdrant, or disabled", nil)
		return
	}
	result, err := s.indexer.Migrate(r.Context(), target)
	if err != nil {
		respondError(w, http.StatusBadGateway, "MIGRATION_FAILED", err.Error(), nil)
		return
	}
	s.searcher.SetVectorStore(target)
	s.cfg.VectorDB.Engine = request.Engine
	if request.Engine == "qdrant" {
		s.cfg.VectorDB.Qdrant.Collection = activeCollection
	}
	if err := config.SaveSemanticSettings(config.SemanticSettings{Engine: request.Engine, QdrantCollection: s.cfg.VectorDB.Qdrant.Collection}); err != nil {
		respondError(w, http.StatusInternalServerError, "SETTINGS_WRITE_FAILED", err.Error(), nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"engine": request.Engine, "collection": activeCollection, "status": "completed", "result": result,
	})
}

var qdrantGenerationSuffix = regexp.MustCompile(`-\d{8}t\d{6}$`)

func qdrantBaseCollection(collection string) string {
	return qdrantGenerationSuffix.ReplaceAllString(strings.TrimSpace(collection), "")
}
