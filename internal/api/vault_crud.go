package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	semcontext "github.com/saterdoe/oberth/internal/context"
)

type vaultNoteRequest struct {
	Path     string         `json:"path"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

func (s *Server) handleCreateVaultNote(w http.ResponseWriter, r *http.Request) {
	var req vaultNoteRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respondError(w, 400, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	if req.Path == "" {
		respondError(w, 400, "VALIDATION_ERROR", "path is required", nil)
		return
	}
	note, err := s.vaultConn.CreateNote(req.Path, req.Content, req.Metadata)
	if err != nil {
		respondError(w, 400, "VAULT_WRITE_ERROR", err.Error(), nil)
		return
	}
	s.refreshVault(req.Path, "created")
	respondJSON(w, http.StatusCreated, note)
}
func (s *Server) handleUpdateVaultNote(w http.ResponseWriter, r *http.Request) {
	var req vaultNoteRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respondError(w, 400, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	path := strings.TrimSpace(r.PathValue("path"))
	if path == "" {
		respondError(w, 400, "VALIDATION_ERROR", "path is required", nil)
		return
	}
	note, err := s.vaultConn.UpdateNote(path, req.Content, req.Metadata)
	if err != nil {
		respondError(w, 400, "VAULT_WRITE_ERROR", err.Error(), nil)
		return
	}
	s.refreshVault(path, "updated")
	respondJSON(w, 200, note)
}
func (s *Server) handleDeleteVaultNote(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.PathValue("path"))
	if path == "" {
		respondError(w, 400, "VALIDATION_ERROR", "path is required", nil)
		return
	}
	if err := s.vaultConn.DeleteNote(path); err != nil {
		respondError(w, 400, "VAULT_WRITE_ERROR", err.Error(), nil)
		return
	}
	s.refreshVault(path, "deleted")
	respondJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) refreshVault(path, action string) {
	notes, err := s.vaultConn.ListAllNotes()
	if err == nil {
		filtered := notes[:0]
		for _, note := range notes {
			if note.Path != "memory-index" {
				filtered = append(filtered, note)
			}
		}
		_, _ = s.vaultConn.UpsertNote("memory-index", semcontext.BuildMemoryIndex(filtered), map[string]any{
			"type": "memory_index", "date": time.Now().UTC().Format(time.RFC3339),
		})
	}
	s.contextCache.Clear()
	if s.indexer != nil {
		go func() { _, _ = s.indexer.ReindexIncremental(context.Background()) }()
	}
	s.broadcastEvent(Event{Type: EventVaultChange, AggregateID: path, Payload: map[string]any{"path": path, "action": action}})
}
func (s *Server) handleCheckVault(w http.ResponseWriter, r *http.Request) {
	report, err := s.vaultConn.CheckIntegrity()
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to check vault", nil)
		return
	}
	respondJSON(w, 200, report)
}
