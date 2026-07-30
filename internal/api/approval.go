package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/permission"
)

func (s *Server) handleListApprovalGates(w http.ResponseWriter, r *http.Request) {
	gates, err := s.approvalGates.List(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	if gates == nil {
		gates = []repos.ApprovalGate{}
	}
	respondJSON(w, http.StatusOK, gates)
}

func (s *Server) resolveApproval(ctx context.Context, request permission.Request) (permission.Decision, bool) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return permission.Ask, false
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	var decision, scope string
	err = tx.QueryRow(ctx, `
		SELECT id,decision,scope FROM approval_resolutions
		WHERE operation=$1 AND target=$2 AND (expires_at IS NULL OR expires_at>NOW())
		  AND ((scope='once' AND run_id=NULLIF($3,'')::uuid)
		    OR (scope='session' AND session_id=NULLIF($4,'')::uuid)
		    OR (scope='project' AND repository_path=COALESCE(
		          (SELECT base_repository FROM task_runs WHERE id=NULLIF($3,'')::uuid),$5)))
		ORDER BY CASE scope WHEN 'once' THEN 1 WHEN 'session' THEN 2 ELSE 3 END,created_at DESC
		LIMIT 1 FOR UPDATE`,
		request.Operation, request.Target, request.RunID, request.SessionID, request.RepoPath).Scan(&id, &decision, &scope)
	if err != nil {
		return permission.Ask, false
	}
	if scope == "once" {
		if _, err := tx.Exec(ctx, `DELETE FROM approval_resolutions WHERE id=$1`, id); err != nil {
			return permission.Ask, false
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return permission.Ask, false
	}
	if strings.EqualFold(decision, "allow") {
		return permission.Allow, true
	}
	return permission.Deny, true
}

func (s *Server) handleCreateApprovalGate(w http.ResponseWriter, r *http.Request) {
	var gate repos.ApprovalGate
	if err := json.NewDecoder(r.Body).Decode(&gate); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if gate.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if err := s.approvalGates.Create(r.Context(), &gate); err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}
	respondJSON(w, http.StatusCreated, gate)
}

func (s *Server) handleUpdateApprovalGate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid gate ID", nil)
		return
	}
	var gate repos.ApprovalGate
	if err := json.NewDecoder(r.Body).Decode(&gate); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if gate.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if err := s.approvalGates.Update(r.Context(), id, &gate); err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
		return
	}
	respondJSON(w, http.StatusOK, gate)
}

func (s *Server) handleDeleteApprovalGate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid gate ID", nil)
		return
	}
	if err := s.approvalGates.Delete(r.Context(), id); err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type checkApprovalRequest struct {
	RepoPath string `json:"repo_path"`
	TaskType string `json:"task_type"`
}

func (s *Server) handleCheckApprovalGate(w http.ResponseWriter, r *http.Request) {
	var req checkApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	gate, err := s.approvalGates.Match(r.Context(), req.RepoPath, req.TaskType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error(), nil)
		return
	}

	resp := map[string]interface{}{
		"requires_approval": false,
		"requires_review":   false,
		"deny_cloud":        false,
		"requires_tests":    false,
	}

	if gate != nil {
		resp["requires_approval"] = gate.RequireApproval
		resp["requires_review"] = gate.RequireReview
		resp["deny_cloud"] = gate.DenyCloud
		resp["requires_tests"] = gate.RequireTests
		resp["gate"] = gate.Name
		resp["gate_id"] = gate.ID.String()
		if gate.MaxCost != nil {
			resp["max_cost"] = *gate.MaxCost
		}
	}

	respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleResolveApproval(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdempotencyKey string `json:"idempotency_key"`
		Scope          string `json:"scope"`
		Decision       string `json:"decision"`
		Operation      string `json:"operation"`
		Target         string `json:"target"`
		UserID         string `json:"user_id"`
		TaskID         string `json:"task_id"`
		SessionID      string `json:"session_id"`
		RunID          string `json:"run_id"`
		RepositoryPath string `json:"repository_path"`
		Risk           string `json:"risk"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.IdempotencyKey == "" ||
		(req.Scope != "once" && req.Scope != "session" && req.Scope != "project") ||
		(req.Decision != "allow" && req.Decision != "deny") || req.Operation == "" {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid approval resolution", nil)
		return
	}
	id := uuid.New()
	var storedID uuid.UUID
	err := s.pool.QueryRow(r.Context(), `
		INSERT INTO approval_resolutions
		  (id,idempotency_key,scope,decision,operation,target,user_id,task_id,session_id,run_id,repository_path,risk,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid,NULLIF($10,'')::uuid,$11,$12,
		        CASE WHEN $3='once' THEN NOW()+INTERVAL '10 minutes' ELSE NULL END)
		ON CONFLICT (idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
		RETURNING id`, id, req.IdempotencyKey, req.Scope, req.Decision, req.Operation, req.Target,
		req.UserID, req.TaskID, req.SessionID, req.RunID, req.RepositoryPath, req.Risk).Scan(&storedID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to persist approval", nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"id": storedID, "scope": req.Scope, "decision": req.Decision})
}
