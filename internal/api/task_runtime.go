package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/saterdoe/oberth/internal/agentprofile"
	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/internal/blockreason"
	"github.com/saterdoe/oberth/internal/codeindex"
	semcontext "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/cost"
	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/gateway"
	"github.com/saterdoe/oberth/internal/permission"
	"github.com/saterdoe/oberth/internal/reasoning"
	"github.com/saterdoe/oberth/internal/tasktype"
	workspacepkg "github.com/saterdoe/oberth/internal/workspace"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
	"github.com/saterdoe/oberth/pkg/llm"
	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

var taskTransitions = map[string]map[string]bool{
	"pending":   {"running": true, "cancelled": true},
	"running":   {"review": true, "completed": true, "blocked": true, "cancelled": true, "failed": true},
	"review":    {"running": true, "completed": true, "cancelled": true},
	"blocked":   {"running": true, "cancelled": true},
	"failed":    {"running": true, "cancelled": true},
	"completed": {"running": true}, "cancelled": {},
}

type taskRunResponse struct {
	TaskID    uuid.UUID `json:"task_id"`
	SessionID uuid.UUID `json:"session_id"`
	RunID     uuid.UUID `json:"run_id"`
	Status    string    `json:"status"`
}

type taskExecutionStage struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

type taskExecutionPlan struct {
	ExecutionPlan []taskExecutionStage `json:"execution_plan"`
}

func isReadOnlyRequest(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(normalized, "?") || strings.Contains(normalized, "¿") {
		return true
	}
	for _, prefix := range []string{"where ", "how ", "what ", "which ", "explain ", "donde ", "dónde ", "como ", "cómo ", "que ", "qué ", "cual ", "cuál "} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func taskInteractionMode(constraints json.RawMessage) string {
	var value struct {
		InteractionMode string `json:"interaction_mode"`
	}
	if json.Unmarshal(constraints, &value) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(value.InteractionMode))
}

func taskHasApprovedPlan(constraints json.RawMessage) bool {
	var value struct {
		ApprovedPlan string `json:"approved_plan"`
	}
	return json.Unmarshal(constraints, &value) == nil && strings.TrimSpace(value.ApprovedPlan) != ""
}

func modelTaskConstraints(constraints json.RawMessage, interactionMode string) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(constraints, &value) != nil {
		return constraints
	}
	messages, _ := value["conversation"].([]any)
	limit := 12
	if interactionMode == "implementation" {
		limit = 4
	}
	if len(messages) > limit {
		value["conversation"] = messages[len(messages)-limit:]
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return constraints
	}
	return normalized
}

func parseTaskExecutionPlan(raw json.RawMessage) []taskExecutionStage {
	var plan taskExecutionPlan
	if len(raw) == 0 || json.Unmarshal(raw, &plan) != nil {
		return nil
	}
	result := make([]taskExecutionStage, 0, len(plan.ExecutionPlan))
	for _, stage := range plan.ExecutionPlan {
		stage.ID = strings.TrimSpace(stage.ID)
		stage.Role = strings.ToLower(strings.TrimSpace(stage.Role))
		stage.ProviderID = strings.TrimSpace(stage.ProviderID)
		stage.Model = strings.TrimSpace(stage.Model)
		if stage.ID != "" && stage.Role != "" && stage.ProviderID != "" && stage.Model != "" {
			result = append(result, stage)
		}
	}
	return result
}

func validateTaskExecutionPlan(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		// Arrays are the legacy free-form constraints representation.
		var legacy []any
		if json.Unmarshal(raw, &legacy) == nil {
			return nil
		}
		return errors.New("constraints must be an array or an execution plan")
	}
	encoded, present := envelope["execution_plan"]
	if !present {
		return nil
	}
	var stages []taskExecutionStage
	if err := json.Unmarshal(encoded, &stages); err != nil || len(stages) == 0 {
		return errors.New("execution_plan must contain at least one stage")
	}
	for i, stage := range stages {
		if strings.TrimSpace(stage.ID) == "" || strings.TrimSpace(stage.Role) == "" ||
			strings.TrimSpace(stage.ProviderID) == "" || strings.TrimSpace(stage.Model) == "" {
			return fmt.Errorf("execution_plan stage %d requires id, role, provider_id and model", i+1)
		}
	}
	return nil
}

func providerOffersModel(provider *repos.Provider, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	if strings.TrimSpace(provider.Models) == "" {
		return model == strings.TrimSpace(provider.DefaultModel)
	}
	for _, candidate := range strings.Split(provider.Models, ",") {
		if strings.TrimSpace(candidate) == model {
			return true
		}
	}
	return false
}

func primaryExecutionStage(stages []taskExecutionStage) *taskExecutionStage {
	for i := range stages {
		if stages[i].Role == "development" {
			return &stages[i]
		}
	}
	if len(stages) > 0 {
		return &stages[0]
	}
	return nil
}

type startingRun struct {
	idempotencyKey string
	done           chan struct{}
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryID string          `json:"repository_id"`
		Title        string          `json:"title"`
		Description  string          `json:"description"`
		TaskType     string          `json:"task_type"`
		Constraints  json.RawMessage `json:"constraints"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respondError(w, 400, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		respondError(w, 400, "VALIDATION_ERROR", "title is required", nil)
		return
	}
	if strings.TrimSpace(req.RepositoryID) == "" {
		respondError(w, 400, "REPOSITORY_REQUIRED", "repository_id is required; oberth never chooses a repository implicitly", nil)
		return
	}
	id, err := uuid.Parse(req.RepositoryID)
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid repository_id", nil)
		return
	}
	repositoryID := &id
	var repositoryExists bool
	if err := s.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, id).Scan(&repositoryExists); err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to validate repository", nil)
		return
	}
	if !repositoryExists {
		respondError(w, 404, "REPOSITORY_NOT_FOUND", "the selected repository is no longer registered; choose it again", nil)
		return
	}
	if req.TaskType == "" {
		req.TaskType = tasktype.Infer(req.Description)
	} else {
		req.TaskType = tasktype.Normalize(req.TaskType)
	}
	if len(req.Constraints) == 0 {
		req.Constraints = json.RawMessage(`[]`)
	}
	if err := validateTaskExecutionPlan(req.Constraints); err != nil {
		respondError(w, 400, "INVALID_EXECUTION_PLAN", err.Error(), nil)
		return
	}
	t := &repos.Task{RepositoryID: repositoryID, Title: strings.TrimSpace(req.Title), Description: strings.TrimSpace(req.Description), TaskType: req.TaskType, Risk: tasktype.InferRisk(req.Description), Strategy: tasktype.InferStrategy(req.Description), Constraints: req.Constraints, Status: "pending"}
	if err := s.tasks.Create(r.Context(), t); err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to create task", nil)
		return
	}
	respondJSON(w, http.StatusCreated, t)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid task ID", nil)
		return
	}
	t, err := s.tasks.GetByID(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		respondError(w, 404, "NOT_FOUND", "task not found", nil)
		return
	}
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to get task", nil)
		return
	}
	respondJSON(w, 200, t)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.tasks.List(r.Context(), repos.TaskFilter{Status: r.URL.Query().Get("status"), Offset: parseIntParam(r.URL.Query().Get("offset"), 0), Limit: parseIntParam(r.URL.Query().Get("limit"), 100)})
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to list tasks", nil)
		return
	}
	respondJSON(w, 200, map[string]any{"tasks": tasks, "total": len(tasks)})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid task ID", nil)
		return
	}
	t, err := s.tasks.GetByID(r.Context(), id)
	if err != nil {
		respondError(w, 404, "NOT_FOUND", "task not found", nil)
		return
	}
	var req struct {
		Title       string          `json:"title"`
		Description string          `json:"description"`
		TaskType    string          `json:"task_type"`
		Status      string          `json:"status"`
		Constraints json.RawMessage `json:"constraints"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		respondError(w, 400, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	if req.Status != "" && req.Status != t.Status && !taskTransitions[t.Status][req.Status] {
		respondError(w, 409, "INVALID_TRANSITION", "task state transition is not allowed", nil)
		return
	}
	if req.Title != "" {
		t.Title = strings.TrimSpace(req.Title)
	}
	if req.Description != "" {
		t.Description = req.Description
	}
	if req.TaskType != "" {
		t.TaskType = tasktype.Normalize(req.TaskType)
	}
	if req.Status != "" {
		t.Status = req.Status
	}
	if len(req.Constraints) > 0 {
		if err := validateTaskExecutionPlan(req.Constraints); err != nil {
			respondError(w, 400, "INVALID_EXECUTION_PLAN", err.Error(), nil)
			return
		}
		t.Constraints = req.Constraints
	}
	if err := s.tasks.Update(r.Context(), t); err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to update task", nil)
		return
	}
	respondJSON(w, 200, t)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid task ID", nil)
		return
	}
	s.runsMu.Lock()
	_, running := s.activeRuns[id]
	s.runsMu.Unlock()
	if running {
		respondError(w, 409, "TASK_RUNNING", "cancel the task before deleting it", nil)
		return
	}
	if err := s.tasks.Delete(r.Context(), id); errors.Is(err, db.ErrNotFound) {
		respondError(w, 404, "NOT_FOUND", "task not found", nil)
		return
	} else if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to delete task", nil)
		return
	}
	respondJSON(w, 200, map[string]string{"status": "deleted"})
}

func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid task ID", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) > 200 {
		respondError(w, 400, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must not exceed 200 characters", nil)
		return
	}
	if idempotencyKey != "" {
		existing, err := s.findIdempotentRun(r.Context(), id, idempotencyKey)
		if err == nil {
			respondJSON(w, http.StatusOK, *existing)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			respondError(w, 500, "RUN_STATE_FAILED", "failed to resolve idempotent run", nil)
			return
		}
	}
	s.runsMu.Lock()
	if start, exists := s.startingRuns[id]; exists {
		if idempotencyKey != "" && start.idempotencyKey == idempotencyKey {
			s.runsMu.Unlock()
			select {
			case <-start.done:
				existing, lookupErr := s.findIdempotentRun(r.Context(), id, idempotencyKey)
				if lookupErr == nil {
					respondJSON(w, http.StatusOK, *existing)
					return
				}
				if !errors.Is(lookupErr, pgx.ErrNoRows) {
					respondError(w, 500, "RUN_STATE_FAILED", "failed to resolve idempotent run", nil)
					return
				}
				respondError(w, 409, "RUN_START_FAILED", "the matching run did not start; retry the request", nil)
			case <-r.Context().Done():
				respondError(w, 408, "REQUEST_CANCELLED", "request cancelled while waiting for matching run", nil)
			}
			return
		}
		s.runsMu.Unlock()
		respondError(w, 409, "TASK_ALREADY_RUNNING", "task is already starting or running", nil)
		return
	}
	if s.activeRuns[id] != nil {
		s.runsMu.Unlock()
		if idempotencyKey != "" {
			existing, lookupErr := s.findIdempotentRun(r.Context(), id, idempotencyKey)
			if lookupErr == nil {
				respondJSON(w, http.StatusOK, *existing)
				return
			}
			if !errors.Is(lookupErr, pgx.ErrNoRows) {
				respondError(w, 500, "RUN_STATE_FAILED", "failed to resolve idempotent run", nil)
				return
			}
		}
		respondError(w, 409, "TASK_ALREADY_RUNNING", "task is already starting or running", nil)
		return
	}
	start := startingRun{idempotencyKey: idempotencyKey, done: make(chan struct{})}
	s.startingRuns[id] = start
	s.runsMu.Unlock()
	defer func() {
		s.runsMu.Lock()
		delete(s.startingRuns, id)
		close(start.done)
		s.runsMu.Unlock()
	}()
	t, err := s.tasks.GetByID(r.Context(), id)
	if err != nil {
		respondError(w, 404, "NOT_FOUND", "task not found", nil)
		return
	}
	if !taskTransitions[t.Status]["running"] {
		respondError(w, 409, "INVALID_TRANSITION", "task cannot be run from its current state", nil)
		return
	}
	if s.executor == nil || s.providers == nil {
		respondError(w, 503, "RUNTIME_UNAVAILABLE", "LLM runtime is not configured", nil)
		return
	}
	repoPath := s.taskRepositoryPath(r.Context(), t.RepositoryID)
	providerID, model, routeRuleID, err := s.selectTaskRoute(r.Context(), t.TaskType, repoPath)
	if err != nil {
		respondError(w, 500, "ROUTING_FAILED", "failed to select provider route", nil)
		return
	}
	if providerID == uuid.Nil {
		respondError(w, 404, "NO_PROVIDER", "no active provider found", nil)
		return
	}
	executionStages := parseTaskExecutionPlan(t.Constraints)
	for _, stage := range executionStages {
		stageProviderID, parseErr := uuid.Parse(stage.ProviderID)
		if parseErr != nil {
			respondError(w, 400, "INVALID_EXECUTION_PLAN", "execution plan contains an invalid provider", nil)
			return
		}
		candidate, providerErr := s.providers.GetByID(r.Context(), stageProviderID)
		if providerErr != nil || !candidate.IsActive {
			respondError(w, 400, "INVALID_EXECUTION_PLAN", "execution plan references an unavailable provider", nil)
			return
		}
		if !providerOffersModel(candidate, stage.Model) {
			respondError(w, 409, "MODEL_NOT_AVAILABLE", fmt.Sprintf("model %q is no longer available from %s; refresh models and choose another", stage.Model, candidate.Name), nil)
			return
		}
	}
	if primary := primaryExecutionStage(executionStages); primary != nil {
		selectedProvider, parseErr := uuid.Parse(primary.ProviderID)
		if parseErr != nil {
			respondError(w, 400, "INVALID_EXECUTION_PLAN", "execution plan contains an invalid provider", nil)
			return
		}
		candidate, providerErr := s.providers.GetByID(r.Context(), selectedProvider)
		if providerErr != nil || !candidate.IsActive {
			respondError(w, 400, "INVALID_EXECUTION_PLAN", "execution plan references an unavailable provider", nil)
			return
		}
		providerID, model, routeRuleID = selectedProvider, primary.Model, "task-explicit"
	}
	if repoPath == "" {
		respondError(w, 400, "WORKSPACE_REQUIRED", "task must reference a registered repository", nil)
		return
	}
	s.cleanupSupersededTaskWorktrees(r.Context(), t.ID)
	sessionID := uuid.New()
	worktreesDir, err := filepath.Abs(filepath.Join("data", "worktrees"))
	if err != nil {
		respondError(w, 500, "WORKTREE_FAILED", "failed to resolve worktree directory", nil)
		return
	}
	worktree, err := gitpkg.CreateSessionWorktreeContext(r.Context(), repoPath, worktreesDir, sessionID.String())
	if err != nil {
		respondError(w, 409, "WORKTREE_FAILED", err.Error(), nil)
		return
	}
	sessionRepoPath := worktree.Path
	sessionBranch := worktree.Branch
	session := &repos.Session{ID: sessionID, TaskID: &t.ID, RepoPath: &sessionRepoPath, Branch: &sessionBranch, TaskType: t.TaskType, TaskDescription: &t.Description, ProviderID: &providerID, Model: &model, Status: "active"}
	if err := s.sessions.Create(r.Context(), session); err != nil {
		_ = gitpkg.CleanupSessionWorktree(worktree, true)
		respondError(w, 500, "INTERNAL_ERROR", "failed to create session", nil)
		return
	}
	baseCommit, err := gitpkg.CurrentCommit(repoPath)
	if err != nil {
		_ = s.sessions.Delete(r.Context(), session.ID)
		_ = gitpkg.CleanupSessionWorktree(worktree, true)
		respondError(w, 500, "WORKTREE_FAILED", "failed to resolve base commit", nil)
		return
	}
	run, err := s.createDurableRun(r.Context(), t.ID, session.ID, repoPath, baseCommit, worktree.Path, worktree.Branch, idempotencyKey)
	if err != nil {
		_ = s.sessions.Delete(r.Context(), session.ID)
		_ = gitpkg.CleanupSessionWorktree(worktree, true)
		respondError(w, 500, "RUN_STATE_FAILED", "failed to persist run state", nil)
		return
	}
	if err := s.tasks.SetStatus(r.Context(), t.ID, []string{t.Status}, "running"); err != nil {
		_ = gitpkg.CleanupSessionWorktree(worktree, true)
		respondError(w, 409, "TASK_ALREADY_RUNNING", "task is already running", nil)
		return
	}
	s.logAudit(r.Context(), &session.ID, "task_route_selected", "agent:task-runtime", map[string]any{
		"task_id":     t.ID.String(),
		"task_type":   t.TaskType,
		"repo_path":   worktree.Path,
		"base_repo":   repoPath,
		"branch":      worktree.Branch,
		"provider_id": providerID.String(),
		"model":       model,
		"rule_id":     routeRuleID,
	})
	runCtx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	s.runsMu.Lock()
	s.activeRuns[t.ID] = cancel
	s.runsMu.Unlock()
	s.broadcastEvent(Event{Type: EventTaskStarted, AggregateID: session.ID.String(), Payload: map[string]any{"task_id": t.ID.String(), "session_id": session.ID.String(), "status": "running"}})
	go s.executeSingleTask(runCtx, run, t, session, providerID.String(), model)
	respondJSON(w, http.StatusAccepted, taskRunResponse{TaskID: t.ID, SessionID: session.ID, RunID: run.ID, Status: "running"})
}

func (s *Server) findIdempotentRun(ctx context.Context, taskID uuid.UUID, idempotencyKey string) (*taskRunResponse, error) {
	var existing taskRunResponse
	err := s.pool.QueryRow(ctx, `
		SELECT task_id,session_id,id,state
		FROM task_runs
		WHERE task_id=$1 AND idempotency_key=$2`, taskID, idempotencyKey).
		Scan(&existing.TaskID, &existing.SessionID, &existing.RunID, &existing.Status)
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *Server) cleanupSupersededTaskWorktrees(ctx context.Context, taskID uuid.UUID) {
	rows, err := s.pool.Query(ctx, `
		SELECT base_repository,worktree_path,branch FROM task_runs
		WHERE task_id=$1 AND state IN ('blocked','failed','cancelled')`, taskID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var worktree gitpkg.SessionWorktree
		if rows.Scan(&worktree.Repository, &worktree.Path, &worktree.Branch) == nil {
			_ = gitpkg.CleanupSessionWorktree(worktree, true)
		}
	}
}

func (s *Server) taskRepositoryPath(ctx context.Context, repositoryID *uuid.UUID) string {
	if repositoryID == nil || s.pool == nil {
		return ""
	}
	var path string
	if err := s.pool.QueryRow(ctx, `SELECT path FROM projects WHERE id = $1`, *repositoryID).Scan(&path); err != nil {
		return ""
	}
	return strings.TrimSpace(path)
}

func (s *Server) selectTaskRoute(ctx context.Context, taskType, repoPath string) (uuid.UUID, string, string, error) {
	if s.router != nil {
		result, err := s.router.Match(ctx, gateway.RouteRequest{TaskType: taskType, RepoPath: repoPath})
		if err != nil {
			return uuid.Nil, "", "", err
		}
		if result != nil && result.Provider != nil && result.Provider.IsActive {
			model := result.Provider.DefaultModel
			ruleID := ""
			if result.Rule != nil {
				ruleID = result.Rule.ID.String()
				if strings.TrimSpace(result.Rule.Model) != "" {
					model = result.Rule.Model
				}
			}
			return result.Provider.ID, model, ruleID, nil
		}
	}
	providers, err := s.providers.List(ctx)
	if err != nil {
		return uuid.Nil, "", "", err
	}
	for _, provider := range providers {
		if provider.IsActive {
			return provider.ID, provider.DefaultModel, "", nil
		}
	}
	return uuid.Nil, "", "", nil
}

func (s *Server) executeSingleTask(ctx context.Context, run *durableRun, task *repos.Task, session *repos.Session, providerID, model string) {
	defer func() { s.runsMu.Lock(); delete(s.activeRuns, task.ID); s.runsMu.Unlock() }()
	runState := "failed"
	runWarnings := []string{}
	runtimeEvidence := map[string]any{}
	var reasoningCase *reasoning.CaseV1
	durableCtx := context.WithoutCancel(ctx)
	defer func() {
		finalizeCtx, cancel := context.WithTimeout(durableCtx, 10*time.Second)
		defer cancel()
		diffs, diffErr := gitpkg.GetDiff(run.WorktreePath)
		if diffErr != nil {
			runWarnings = append(runWarnings, diffErr.Error())
		}
		diffBytes, _ := json.Marshal(diffs)
		diffHash := fmt.Sprintf("sha256:%x", sha256.Sum256(diffBytes))
		refreshReasoningEvidence(run.WorktreePath, diffHash, reasoningCase)
		contextHash := fmt.Sprintf("sha256:%x", sha256.Sum256(session.ContextUsed))
		s.finishDurableRun(finalizeCtx, run.ID, runState, ResultBundleV1{
			SchemaVersion: resultBundleSchemaVersion,
			RunID:         run.ID, TaskID: task.ID, SessionID: session.ID,
			BaseCommit: run.BaseCommit, Worktree: run.WorktreePath, Branch: run.Branch,
			Diff: diffs, DiffHash: diffHash,
			Context: session.ContextUsed, ContextHash: contextHash,
			TokensInput: session.TokensInput, TokensOutput: session.TokensOutput, Cost: session.Cost,
			Warnings: runWarnings, Commands: runtimeEvidence["commands"],
			VerificationStatus: runtimeEvidence["verification_status"], Runtime: runtimeEvidence,
			Environment: EnvironmentV1{OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version()},
			Reasoning:   reasoningCase,
		})
	}()
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go func() {
		ticker := time.NewTicker(runLeaseDuration / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !s.heartbeatRun(heartbeatCtx, run) {
					return
				}
			case <-heartbeatCtx.Done():
				return
			}
		}
	}()
	interactionMode := taskInteractionMode(task.Constraints)
	prompt := secretspkg.Redact(agentprofile.BuildTaskPrompt(task.Title, task.Description, modelTaskConstraints(task.Constraints, interactionMode)))
	readOnlyRequest := interactionMode == "query" || interactionMode == "plan" || (interactionMode == "" && isReadOnlyRequest(task.Title+" "+task.Description))
	contextPipeline := semcontext.NewPipeline(s.vaultConn, s.searcher)
	retrievalMetrics := semcontext.RetrievalMetrics{}
	compileOptions := semcontext.CompileOptions{
		Mode:                  semcontext.ModeDev,
		MaxTokens:             s.cfg.Context.MaxTokens,
		ReserveOutputTokens:   s.cfg.Context.ReserveOutputTokens,
		MaxSourcesPerKind:     s.cfg.Context.MaxSourcesPerKind,
		Cache:                 s.contextCache,
		DisableCodeIndex:      !s.cfg.CodeIndex.IsEnabled(),
		CodeIndex:             codeindex.Options{MaxFileBytes: s.cfg.CodeIndex.MaxFileBytes, MaxFiles: s.cfg.CodeIndex.MaxFiles, MaxChunks: s.cfg.CodeIndex.MaxChunks, MaxChunkLines: s.cfg.CodeIndex.MaxChunkLines, OverlapLines: s.cfg.CodeIndex.OverlapLines, Exclude: s.cfg.CodeIndex.Exclude},
		CodeIndexIdentityRoot: run.BaseRepo,
	}
	modelOutputTokens := 0
	localOllama := false
	if id, parseErr := uuid.Parse(providerID); parseErr == nil {
		if selected, providerErr := s.providers.GetByID(ctx, id); providerErr == nil {
			compileOptions = contextOptionsForProvider(compileOptions, selected.ProviderType)
			if strings.EqualFold(strings.TrimSpace(selected.ProviderType), "ollama") {
				localOllama = true
				modelOutputTokens = 800
				if readOnlyRequest {
					modelOutputTokens = 400
				}
			}
		}
	}
	if s.searcher != nil {
		compileOptions.Expand = func(expandCtx context.Context, _ int) ([]semcontext.ContextSource, error) {
			results, metrics, err := s.searcher.SearchWithMetrics(expandCtx, task.Description, 12, "")
			retrievalMetrics = metrics
			if err != nil {
				return nil, err
			}
			sources := make([]semcontext.ContextSource, 0, len(results))
			for _, result := range results {
				scope, _ := result.Note.Metadata["scope"].(string)
				if scope != "" && !strings.EqualFold(filepath.Clean(scope), filepath.Clean(run.BaseRepo)) {
					continue
				}
				content := result.Excerpt
				if strings.TrimSpace(content) == "" {
					content = result.Note.Content
				}
				sources = append(sources, semcontext.ContextSource{
					ID: result.Note.Path, Kind: "memory", Content: content,
					Relevance: float64(result.Score), Reason: result.Reason,
				})
			}
			return sources, nil
		}
	}
	compiled, compileErr := contextPipeline.CompileRepository(ctx, run.WorktreePath, task.Description, task.TaskType, compileOptions)
	contextEnvelopeFingerprint := ""
	if compileErr != nil {
		runWarnings = append(runWarnings, "context compilation: "+compileErr.Error())
	} else {
		redactedConstraints := json.RawMessage(secretspkg.Redact(string(task.Constraints)))
		if len(redactedConstraints) > 0 && !json.Valid(redactedConstraints) {
			redactedConstraints, _ = json.Marshal(secretspkg.Redact(string(task.Constraints)))
		}
		envelope, envelopeErr := semcontext.NewContextEnvelopeV1(
			semcontext.ContextEnvelopeTaskV1{
				ID: task.ID.String(), Title: secretspkg.Redact(task.Title), Description: secretspkg.Redact(task.Description),
				Type: task.TaskType, Constraints: redactedConstraints,
			},
			semcontext.ContextEnvelopeRepoV1{Identity: run.BaseRepo, BaseCommit: run.BaseCommit},
			compiled,
			retrievalMetrics,
		)
		if envelopeErr != nil {
			runWarnings = append(runWarnings, "context envelope: "+envelopeErr.Error())
		} else {
			manifest, marshalErr := json.Marshal(envelope)
			if marshalErr != nil {
				runWarnings = append(runWarnings, "context envelope serialization: "+marshalErr.Error())
			} else {
				session.ContextUsed = manifest
				contextEnvelopeFingerprint = envelope.Fingerprint
				prompt += "\n\n## Context envelope\nSchema: " + envelope.SchemaVersion + "\nFingerprint: " + envelope.Fingerprint + "\nTreat the task, repository identity, base commit, and constraints associated with this fingerprint as immutable across workflow stages."
			}
		}
		if strings.TrimSpace(compiled.Context) != "" {
			prompt += "\n\n## Traced repository context\nEach excerpt begins with its canonical repository path and line range. Markdown headings inside an excerpt are content, never filenames. Only pass an exact canonical path shown at the start of an excerpt to read. The excerpts themselves may be used as evidence without reading them again.\n\n" + secretspkg.Redact(compiled.Context)
		}
		_ = s.appendRunEvent(ctx, run.ID, "context_compiled", map[string]any{
			"envelope_fingerprint": contextEnvelopeFingerprint,
			"manifest":             compiled.Manifest,
			"exclusions":           compiled.Exclusions,
			"metrics":              compiled.Metrics,
		})
	}
	executionStages := parseTaskExecutionPlan(task.Constraints)
	developmentIndex := -1
	for i, stage := range executionStages {
		if stage.Role == "development" {
			developmentIndex = i
			break
		}
	}
	if developmentIndex < 0 && len(executionStages) > 0 {
		developmentIndex = 0
	}
	handoffs := make([]string, 0, len(executionStages))
	for i, stage := range executionStages {
		if i >= developmentIndex {
			break
		}
		output, stageErr := s.runWorkflowAdvisory(ctx, run, task, session, stage, prompt, "", contextEnvelopeFingerprint)
		if stageErr != nil {
			runWarnings = append(runWarnings, fmt.Sprintf("stage %s failed: %v", stage.ID, stageErr))
			_ = s.appendRunEvent(ctx, run.ID, "workflow_stage_failed", map[string]any{"stage": stage, "error": stageErr.Error()})
			continue
		}
		handoffs = append(handoffs, fmt.Sprintf("### %s (%s)\n%s", stage.ID, stage.Role, output))
	}
	if len(handoffs) > 0 {
		prompt += "\n\n## Handoffs from earlier workflow stages\n" + strings.Join(handoffs, "\n\n")
	}
	s.logAudit(ctx, &session.ID, "agent_profile_selected", "agent:task-runtime", map[string]any{
		"profile": agentprofile.SingleTaskVersion,
		"task_id": task.ID.String(),
	})
	typedSystemPrompt := secretspkg.Redact(agentprofile.SingleTaskSystemPrompt) + `

You operate through exactly one typed action per turn. Return JSON only:
{"schema_version":"1","tool":"read|search|patch|record_reasoning|stop_insufficient_evidence|command|finish","arguments":{},"summary":""}
All file paths must be relative to the workspace. Explore before patching.
Do not stop for insufficient evidence on an ordinary implementation request while repository inspection or an allowed tool action can still make progress.
Use read, not shell commands such as cat or ls, to inspect files.
Use record_reasoning for material facts, hypotheses, assumptions, unknowns,
acceptance properties, reproducible experiments and the final decision. Record concise reviewable claims,
not private chain-of-thought. Link claims to evidence IDs when evidence exists.
Experiments preserve question, environment, command, expectation, observation,
duration/cost, evidence IDs, affected claims and optional base/candidate fingerprints.
Successful reads, searches and commands include an automatic evidence.id in
their observation; cite that ID instead of inventing an evidence reference.
An unknown must name the smallest next action that could resolve it.
If an unresolved unknown makes a safe change unjustifiable, use
stop_insufficient_evidence with that unknown_id and a concise summary. This is
a legitimate outcome and does not require a verification command.
Before finish, run at least one allowed verification command. The safe baseline
for every repository is command {"program":"git","args":["diff","--check"]}.
Other allowed verification commands are go test, go vet, npm test,
npm run test, npm run typecheck, npm run build, cargo test, and pytest.
Do not run pytest unless the repository already contains Python tests or Python test configuration.
For a new dependency-free script with no tests, use only git diff --check.
Never call a command outside this allowlist.
To add a file, use patch arguments {"operation":"create","path":"relative/path","new_text":"..."};
otherwise provide old_text that matches exactly once and an optional expected_hash.
The repository map is authoritative for path existence. Do not read a required file that is absent from that map; create it directly with patch operation create.
Use finish only after inspecting the result and include a concise summary.`
	if readOnlyRequest {
		if interactionMode == "plan" {
			typedSystemPrompt = `You produce a proposed implementation plan for a source repository. Use the traced repository context and conversation supplied by the user. Do not modify files. Return a concrete, reviewable plan in prose. Respect the requested plan size and file constraints; do not add tests, files, risks, or extra sections that the user excluded or that are unnecessary for the requested scope. Do not emit JSON or tool calls. The plan will only be implemented after the user approves it.`
		} else {
			typedSystemPrompt = `You answer questions in an ongoing software conversation. Focus on the latest Description and use the conversation history for continuity. For general engineering concepts, use established technical knowledge. For claims about this specific repository, rely only on the traced repository context and cite canonical files; do not invent repository behavior. Answer directly in the language of the latest question. Do not emit JSON, tool calls, patches, plans, or meta-commentary. If repository evidence is incomplete, separate what is generally true from what is actually confirmed here.`
		}
	}
	runtimeEvidence["prompt_contract"] = map[string]any{
		"id": "typed-agent", "version": "1", "schema_version": "1",
		"template_hash": fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(typedSystemPrompt))),
		"capabilities":  []string{"typed_actions", "read", "search", "patch", "record_reasoning", "stop_insufficient_evidence", "command", "finish"},
	}
	runtimeEvidence["interaction_mode"] = map[bool]string{true: "query", false: "implementation"}[readOnlyRequest]
	if interactionMode != "" {
		runtimeEvidence["interaction_mode"] = interactionMode
	}
	availableTools := agentToolDefinitions()
	if localOllama && strings.Contains(strings.ToLower(model), "qwen3") {
		// Qwen3 otherwise spends most of a CPU-only run in hidden deliberation.
		// The switch must appear in the latest user turn as well as the system
		// prompt; Qwen's chat template does not consistently honor it otherwise.
		typedSystemPrompt = "/no_think\n" + typedSystemPrompt
		prompt = "/no_think\n" + prompt
	}
	if readOnlyRequest {
		availableTools = nil
		runtimeEvidence["prompt_contract"].(map[string]any)["capabilities"] = []string{"semantic_context", "direct_answer"}
	} else if localOllama {
		availableTools = localImplementationToolDefinitions()
		runtimeEvidence["prompt_contract"].(map[string]any)["capabilities"] = []string{"typed_actions", "read", "search", "patch", "command", "finish"}
		typedSystemPrompt = strings.ReplaceAll(typedSystemPrompt, "read|search|patch|record_reasoning|stop_insufficient_evidence|command|finish", "read|search|patch|command|finish")
		typedSystemPrompt = strings.ReplaceAll(typedSystemPrompt, `Use record_reasoning for material facts, hypotheses, assumptions, unknowns,
acceptance properties, reproducible experiments and the final decision. Record concise reviewable claims,
not private chain-of-thought. Link claims to evidence IDs when evidence exists.
Experiments preserve question, environment, command, expectation, observation,
duration/cost, evidence IDs, affected claims and optional base/candidate fingerprints.
Successful reads, searches and commands include an automatic evidence.id in
their observation; cite that ID instead of inventing an evidence reference.
An unknown must name the smallest next action that could resolve it.
If an unresolved unknown makes a safe change unjustifiable, use
stop_insufficient_evidence with that unknown_id and a concise summary. This is
a legitimate outcome and does not require a verification command.
`, "")
	}
	runtimeWorkspace, workspaceErr := workspacepkg.NewRuntime(run.ID.String(), run.WorktreePath, s.perm)
	if workspaceErr != nil {
		runWarnings = append(runWarnings, workspaceErr.Error())
		_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "failed")
		session.Status = "error"
		_ = s.sessions.Update(durableCtx, session)
		return
	}
	nativeToolCalls := 0
	structuredTransportFallbacks := 0
	maxIterations := s.cfg.Agent.MaxIterations
	if id, parseErr := uuid.Parse(providerID); parseErr == nil {
		if selected, providerErr := s.providers.GetByID(ctx, id); providerErr == nil && selected.ProviderType == "ollama" && maxIterations < 24 {
			// Small local models often need more short tool turns than cloud models.
			// Ollama is observable and free of per-request quota, so a larger turn
			// budget is safer than declaring a healthy model timed out mid-change.
			maxIterations = 24
		}
	}
	toolApprovalResolver := s.resolveApproval
	if interactionMode == "implementation" && taskHasApprovedPlan(task.Constraints) {
		toolApprovalResolver = func(approvalCtx context.Context, request permission.Request) (permission.Decision, bool) {
			if request.Operation == "file.write" {
				return permission.Allow, true
			}
			return s.resolveApproval(approvalCtx, request)
		}
	}
	agentResult, err := agentruntime.Run(gateway.WithAuditSessionID(ctx, session.ID.String()), typedSystemPrompt, prompt, agentruntime.Config{
		MaxTurns:            maxIterations,
		MaxInputTokens:      s.cfg.Context.MaxTokens * maxIterations,
		MaxOutputTokens:     s.cfg.Context.ReserveOutputTokens * maxIterations,
		MaxToolCalls:        maxIterations - 1,
		MaxFormatRetries:    2,
		MaxProtocolRetries:  2,
		MaxObservationBytes: 2 * 1024 * 1024,
		MaxDuration:         30 * time.Minute,
		Model: func(modelCtx context.Context, messages []agentruntime.Message) (agentruntime.ModelResponse, error) {
			var reservation *cost.Reservation
			if s.costTracker != nil {
				status, budgetErr := s.costTracker.CheckBudget(modelCtx, providerID)
				if budgetErr != nil {
					return agentruntime.ModelResponse{}, budgetErr
				}
				if status != nil && status.IsBlocked {
					return agentruntime.ModelResponse{}, agentruntime.ErrBudgetExceeded
				}
				estimatedInput := 0
				for _, message := range messages {
					estimatedInput += len([]rune(message.Content))/4 + 4
				}
				inputReserve, outputReserve := estimateCallCost(model, estimatedInput, s.cfg.Context.ReserveOutputTokens)
				reservation, budgetErr = s.costTracker.Reserve(modelCtx, providerID, inputReserve+outputReserve)
				if errors.Is(budgetErr, cost.ErrBudgetExceeded) {
					return agentruntime.ModelResponse{}, agentruntime.ErrBudgetExceeded
				}
				if budgetErr != nil {
					return agentruntime.ModelResponse{}, budgetErr
				}
				defer s.costTracker.Release(context.WithoutCancel(modelCtx), reservation)
			}
			llmMessages := make([]llm.Message, 0, len(messages))
			for _, message := range messages {
				llmMessages = append(llmMessages, llm.Message{Role: message.Role, Content: message.Content})
			}
			response, err := s.executor.ExecuteStepStream(modelCtx, gateway.Step{ID: "typed-agent-v1", ProviderID: providerID, Model: model, Tools: availableTools, MaxTokens: modelOutputTokens}, llmMessages, func(content string) {
				s.broadcastTaskChunk(task.ID, session.ID, content)
			})
			if err != nil {
				lower := strings.ToLower(err.Error())
				if strings.Contains(lower, "tool") || strings.Contains(lower, "function") || strings.Contains(lower, "400") {
					structuredTransportFallbacks++
					response, err = s.executor.ExecuteStepStream(modelCtx, gateway.Step{ID: "typed-agent-v1-fallback", ProviderID: providerID, Model: model, MaxTokens: modelOutputTokens}, llmMessages, func(content string) {
						s.broadcastTaskChunk(task.ID, session.ID, content)
					})
				}
			}
			if err != nil {
				return agentruntime.ModelResponse{}, err
			}
			content := response.Content
			if response.ToolCall != nil {
				nativeToolCalls++
				var arguments map[string]any
				if json.Unmarshal(response.ToolCall.Arguments, &arguments) != nil {
					// Let agentruntime send its normal format-correction turn instead
					// of immediately classifying one malformed native call as fatal.
					return agentruntime.ModelResponse{Content: string(response.ToolCall.Arguments), Model: response.Model, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens}, nil
				}
				action := map[string]any{"schema_version": "1", "tool": response.ToolCall.Name, "arguments": arguments}
				if response.ToolCall.Name == "finish" {
					action["summary"] = arguments["summary"]
				}
				encoded, _ := json.Marshal(action)
				content = string(encoded)
			} else if readOnlyRequest && strings.TrimSpace(response.Content) != "" {
				// A direct answer is the expected terminal response for read-only
				// questions. Local OpenAI-compatible models frequently return it as
				// content instead of invoking the finish tool.
				summary := strings.TrimSpace(response.Content)
				encoded, _ := json.Marshal(map[string]any{
					"schema_version": "1",
					"tool":           "finish",
					"arguments":      map[string]any{"summary": summary},
					"summary":        summary,
				})
				content = string(encoded)
			} else if summary, ok := finishAfterSuccessfulVerification(messages, response.Content); ok {
				// Some OpenAI-compatible local reasoning models correctly use
				// native tools, then emit their final answer as plain content
				// instead of a finish tool call. Once a typed verification
				// observation passed, normalize that terminal response into the
				// versioned finish action rather than rejecting a valid run.
				encoded, _ := json.Marshal(map[string]any{
					"schema_version": "1",
					"tool":           "finish",
					"arguments":      map[string]any{"summary": summary},
					"summary":        summary,
				})
				content = string(encoded)
			}
			if s.costTracker != nil {
				inputCost, outputCost := estimateCallCost(response.Model, response.InputTokens, response.OutputTokens)
				_, recordErr := s.costTracker.RecordCall(modelCtx, cost.CallRecord{
					SessionID: session.ID.String(), ProviderID: providerID, Model: response.Model,
					TokensInput: response.InputTokens, TokensOutput: response.OutputTokens,
					CostInput: inputCost, CostOutput: outputCost,
				})
				if recordErr != nil {
					runWarnings = append(runWarnings, "cost accounting: "+recordErr.Error())
				}
				session.Cost += inputCost + outputCost
			}
			return agentruntime.ModelResponse{Content: content, Model: response.Model, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens}, nil
		},
		Execute: func(toolCtx context.Context, action agentruntime.Action) agentruntime.Observation {
			return executeTypedTool(toolCtx, runtimeWorkspace, run.ID.String(), task.ID.String(), session.ID.String(), task.TaskType, task.Risk, s.perm, toolApprovalResolver, action)
		},
		OnTurn: func(turn int, action agentruntime.Action, observation *agentruntime.Observation) {
			attachAutomaticEvidence(turn, action, observation)
			payload := map[string]any{"turn": turn, "action": action}
			if observation != nil {
				payload["observation"] = observation
				if action.Tool == "read" || action.Tool == "search" {
					encoded, _ := json.Marshal(observation.Data)
					payload["context_delta"] = map[string]any{
						"schema_version": "1",
						"operation":      action.Tool,
						"hash":           fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)),
						"bytes":          len(encoded),
					}
				}
			}
			_ = s.appendRunEvent(ctx, run.ID, "agent_turn", payload)
		},
	})
	if readOnlyRequest && compiled != nil && strings.TrimSpace(agentResult.Summary) != "" {
		seenPaths := map[string]bool{}
		paths := make([]string, 0, len(compiled.Manifest))
		for _, source := range compiled.Manifest {
			if source.Kind != "code" && source.Kind != "configuration" && source.Kind != "documentation" {
				continue
			}
			path := strings.SplitN(source.ID, ":", 2)[0]
			if path != "" && !seenPaths[path] {
				seenPaths[path] = true
				paths = append(paths, path)
			}
		}
		if len(paths) > 0 {
			agentResult.Summary += "\n\nRepository sources:\n- `" + strings.Join(paths, "`\n- `") + "`"
		}
	}
	resp := &llm.ChatResponse{
		Model: agentResult.Model, Content: agentResult.Summary,
		InputTokens: agentResult.InputTokens, OutputTokens: agentResult.OutputTokens,
	}
	// Persist usage before evaluating the terminal state so blocked, failed and
	// cancelled runs retain model consumption that already occurred.
	session.TokensInput += resp.InputTokens
	session.TokensOutput += resp.OutputTokens
	if strings.TrimSpace(resp.Model) != "" {
		session.Model = &resp.Model
	}
	runtimeEvidence["turns"] = agentResult.Turns
	runtimeEvidence["actions"] = agentResult.Actions
	runtimeEvidence["json_fallbacks"] = agentResult.JSONFallbacks
	runtimeEvidence["native_tool_calls"] = nativeToolCalls
	runtimeEvidence["structured_transport_fallbacks"] = structuredTransportFallbacks
	runtimeEvidence["action_transport"] = map[bool]string{true: "native-tool-calling", false: "versioned-structured-adapter"}[nativeToolCalls > 0]
	runtimeEvidence["validation"] = map[string]any{
		"schema_version": "1", "valid": err == nil, "fallback_count": agentResult.JSONFallbacks,
	}
	verificationStatus := "not_run"
	commandEvidence := make([]agentruntime.Observation, 0)
	for _, observation := range agentResult.Observations {
		if observation.Tool != "command" {
			continue
		}
		commandEvidence = append(commandEvidence, observation)
		if observation.Status == "ok" {
			verificationStatus = "passed"
		} else {
			verificationStatus = "failed"
		}
	}
	// The runtime owns the delivery gate. A weaker model may edit correctly but
	// choose an unavailable shell command, so always execute the safe,
	// deterministic baseline before deciding whether the run can enter review.
	baselineDiff, baselineDiffErr := gitpkg.GetDiff(runtimeWorkspace.Root())
	if err == nil && verificationStatus != "passed" && baselineDiffErr == nil && len(baselineDiff) > 0 {
		output, verifyErr := gitpkg.CheckDiff(runtimeWorkspace.Root())
		baseline := agentruntime.Observation{
			SchemaVersion: agentruntime.SchemaVersion,
			Tool:          "command",
			Status:        "ok",
			Data: map[string]any{
				"command": "git diff --check",
				"cwd":     runtimeWorkspace.Root(),
				"impact":  "verification only",
				"policy":  "runtime-baseline",
				"result": map[string]any{
					"status": map[bool]string{true: "failed", false: "ok"}[verifyErr != nil],
					"output": output,
				},
			},
		}
		if verifyErr != nil {
			baseline.Status = "failed"
			baseline.Error = verifyErr.Error()
			verificationStatus = "failed"
		} else {
			verificationStatus = "passed"
		}
		attachAutomaticEvidence(len(agentResult.Observations)+1, agentruntime.Action{
			SchemaVersion: agentruntime.SchemaVersion, Tool: "command",
			Arguments: json.RawMessage(`{"program":"git","args":["diff","--check"]}`),
		}, &baseline)
		agentResult.Observations = append(agentResult.Observations, baseline)
		commandEvidence = append(commandEvidence, baseline)
		_ = s.appendRunEvent(durableCtx, run.ID, "verification_baseline", baseline)
	}
	runtimeEvidence["observations"] = agentResult.Observations
	reasoningObservations := make([]reasoning.Observation, 0, len(agentResult.Observations))
	for _, observation := range agentResult.Observations {
		var evidence *reasoning.EvidenceRef
		if observation.Evidence != nil {
			evidence = &reasoning.EvidenceRef{
				ID: observation.Evidence.ID, Source: observation.Evidence.Source,
				Hash: observation.Evidence.Hash, Subject: observation.Evidence.Subject,
				SubjectHash: observation.Evidence.SubjectHash, Detail: observation.Evidence.Detail,
			}
		}
		reasoningObservations = append(reasoningObservations, reasoning.Observation{
			Tool: observation.Tool, Status: observation.Status, Data: observation.Data, Evidence: evidence,
		})
	}
	reasoningCase = reasoning.Collect(reasoningObservations)
	runtimeEvidence["commands"] = commandEvidence
	runtimeEvidence["verification_status"] = verificationStatus
	runtimeEvidence["termination_reason"] = agentResult.Termination
	if reasoningCase != nil && len(reasoningCase.Assessment.DanglingEvidence) > 0 {
		runState = "blocked"
		session.Status = "blocked"
		cause := "reasoning records cite evidence IDs that were not produced by this run: " +
			strings.Join(reasoningCase.Assessment.DanglingEvidence, ", ")
		runWarnings = append(runWarnings, "reasoning_evidence_invalid: "+cause)
		_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "blocked")
		_ = s.sessions.Update(durableCtx, session)
		_ = s.appendRunEvent(durableCtx, run.ID, "run_blocked", blockreason.Block{
			Code: "reasoning_evidence_invalid", Cause: cause,
			NextAction: "retry and cite an evidence.id returned by a successful tool observation", Recoverable: true,
		})
		s.broadcastTaskEvent("blocked", session.ID, "reasoning evidence references must come from this run")
		return
	}
	if agentResult.Termination == "insufficient_evidence" {
		unknown, ok := reasoning.FindUnresolvedUnknown(reasoningCase, agentResult.UnknownID)
		if !ok {
			runState = "failed"
			session.Status = "error"
			runWarnings = append(runWarnings, "invalid_reasoning_stop: referenced unknown was not recorded as unresolved")
			_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "failed")
			_ = s.sessions.Update(durableCtx, session)
			_ = s.appendRunEvent(durableCtx, run.ID, "run_blocked", blockreason.Block{
				Code: "invalid_reasoning_stop", Cause: "the agent stopped without a matching unresolved unknown",
				NextAction: "retry and record the missing unknown before stopping", Recoverable: true,
			})
			return
		}
		runState = "blocked"
		session.Status = "blocked"
		runWarnings = append(runWarnings, "evidence_insufficient: "+unknown.Statement)
		_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "blocked")
		_ = s.sessions.Update(durableCtx, session)
		_ = s.appendRunEvent(durableCtx, run.ID, "run_blocked", blockreason.Block{
			Code: "evidence_insufficient", Cause: unknown.Statement,
			NextAction: unknown.NextAction, Recoverable: true,
		})
		s.broadcastTaskEvent("blocked", session.ID, unknown.NextAction)
		return
	}
	if verificationStatus != "not_run" {
		_, _ = s.pool.Exec(durableCtx, `UPDATE task_runs SET verification_at=COALESCE(verification_at,NOW()) WHERE id=$1`, run.ID)
	}
	now := time.Now()
	session.EndedAt = &now
	if err != nil {
		if errors.Is(err, context.Canceled) {
			runState = "cancelled"
			session.Status = "cancelled"
			_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "cancelled")
			s.broadcastTaskEvent("cancelled", session.ID, task.Title)
		} else {
			block := blockreason.Classify(err)
			runWarnings = append(runWarnings, block.Code+": "+block.Cause)
			_ = s.appendRunEvent(durableCtx, run.ID, "run_blocked", block)
			if block.Recoverable {
				runState = "blocked"
				session.Status = "blocked"
				_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "blocked")
				s.broadcastTaskEvent("blocked", session.ID, block.NextAction)
			} else {
				runState = "failed"
				session.Status = "error"
				_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "failed")
				s.broadcastTaskEvent("failed", session.ID, task.Title)
			}
		}
		_ = s.sessions.Update(durableCtx, session)
		return
	}
	if !readOnlyRequest && verificationStatus != "passed" {
		runState = "blocked"
		session.Status = "blocked"
		cause := "the agent finished without executing a verification command"
		if verificationStatus == "failed" {
			cause = "the final verification command failed"
		}
		runWarnings = append(runWarnings, "tests_failed: "+cause)
		_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "blocked")
		_ = s.sessions.Update(durableCtx, session)
		_ = s.appendRunEvent(durableCtx, run.ID, "run_blocked", blockreason.Block{
			Code: "tests_failed", Cause: cause,
			NextAction: "retry the task and run the repository verification command", Recoverable: true,
		})
		s.broadcastTaskEvent("blocked", session.ID, "verification is required before review")
		return
	}
	if developmentIndex >= 0 && developmentIndex+1 < len(executionStages) {
		currentDiff, diffErr := gitpkg.GetDiff(run.WorktreePath)
		if diffErr != nil {
			runWarnings = append(runWarnings, "workflow review diff: "+diffErr.Error())
		}
		diffJSON, _ := json.Marshal(currentDiff)
		reviewContext := "## Candidate diff\n" + string(diffJSON) + "\n\n## Verification\n" + verificationStatus
		for _, stage := range executionStages[developmentIndex+1:] {
			output, stageErr := s.runWorkflowAdvisory(ctx, run, task, session, stage, prompt, reviewContext, contextEnvelopeFingerprint)
			if stageErr != nil {
				runWarnings = append(runWarnings, fmt.Sprintf("stage %s failed: %v", stage.ID, stageErr))
				_ = s.appendRunEvent(ctx, run.ID, "workflow_stage_failed", map[string]any{"stage": stage, "error": stageErr.Error()})
				continue
			}
			if (stage.Role == "qa" || stage.Role == "review") && strings.Contains(strings.ToUpper(output), "VERDICT: FAIL") {
				runState = "blocked"
				session.Status = "blocked"
				runWarnings = append(runWarnings, fmt.Sprintf("%s rejected the candidate change", stage.Role))
				_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "blocked")
				_ = s.sessions.Update(durableCtx, session)
				_ = s.appendRunEvent(durableCtx, run.ID, "run_blocked", blockreason.Block{
					Code: "workflow_review_failed", Cause: output,
					NextAction: "review the stage feedback and retry with corrections", Recoverable: true,
				})
				return
			}
		}
	}
	if readOnlyRequest {
		session.Status = "completed"
		runState = "completed"
		_ = s.sessions.Update(durableCtx, session)
		_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "completed")
		s.broadcastTaskEvent("completed", session.ID, agentResult.Summary)
		return
	}
	session.Status = "review"
	runState = "review"
	_ = s.sessions.Update(durableCtx, session)
	_ = s.tasks.SetStatus(durableCtx, task.ID, []string{"running"}, "review")

	memoryDiff, _ := gitpkg.GetDiff(run.WorktreePath)
	fallbackMemory := verifiedRunMemoryProposal(run.ID, task.Title, reasoningCase, memoryDiff)
	s.createMemoryCandidates(durableCtx, run, reasoningCase, agentResult.Summary, fallbackMemory)
	s.broadcastTaskEvent("review", session.ID, agentResult.Summary)
}

func contextOptionsForProvider(options semcontext.CompileOptions, providerType string) semcontext.CompileOptions {
	if !strings.EqualFold(strings.TrimSpace(providerType), "ollama") {
		return options
	}
	// Keep a bounded local profile while leaving enough room to benefit from
	// larger Ollama context windows configured by the user.
	if options.MaxTokens <= 0 || options.MaxTokens > 8000 {
		options.MaxTokens = 8000
	}
	if options.ReserveOutputTokens <= 0 || options.ReserveOutputTokens > 1500 {
		options.ReserveOutputTokens = 1500
	}
	if options.MaxSourcesPerKind <= 0 || options.MaxSourcesPerKind > 4 {
		options.MaxSourcesPerKind = 4
	}
	return options
}

func (s *Server) runWorkflowAdvisory(ctx context.Context, run *durableRun, task *repos.Task, session *repos.Session, stage taskExecutionStage, taskPrompt, extraContext, contextEnvelopeFingerprint string) (string, error) {
	system := "You are the " + stage.Role + " stage in a software delivery workflow. Do not claim to have changed files. Produce a concise, actionable handoff for the next stage."
	if stage.Role == "qa" || stage.Role == "review" {
		system += " Evaluate the supplied diff and verification evidence. End with exactly VERDICT: PASS or VERDICT: FAIL."
	}
	_ = s.appendRunEvent(ctx, run.ID, "workflow_stage_started", map[string]any{"stage": stage, "context_envelope_fingerprint": contextEnvelopeFingerprint})
	response, err := s.executor.ExecuteStepStream(
		ctx,
		gateway.Step{ID: stage.ID, ProviderID: stage.ProviderID, Model: stage.Model},
		[]llm.Message{{Role: "system", Content: system}, {Role: "user", Content: taskPrompt + "\n\n" + extraContext}},
		func(content string) { s.broadcastTaskChunk(task.ID, session.ID, "["+stage.Role+"] "+content) },
	)
	if err != nil {
		return "", err
	}
	session.TokensInput += response.InputTokens
	session.TokensOutput += response.OutputTokens
	if s.costTracker != nil {
		inputCost, outputCost := estimateCallCost(response.Model, response.InputTokens, response.OutputTokens)
		_, recordErr := s.costTracker.RecordCall(ctx, cost.CallRecord{
			SessionID: session.ID.String(), ProviderID: stage.ProviderID, Model: response.Model,
			TokensInput: response.InputTokens, TokensOutput: response.OutputTokens,
			CostInput: inputCost, CostOutput: outputCost,
		})
		if recordErr == nil {
			session.Cost += inputCost + outputCost
		}
	}
	output := strings.TrimSpace(response.Content)
	_ = s.appendRunEvent(ctx, run.ID, "workflow_stage_completed", map[string]any{
		"stage": stage, "context_envelope_fingerprint": contextEnvelopeFingerprint,
		"model": response.Model, "tokens_input": response.InputTokens,
		"tokens_output": response.OutputTokens, "output": secretspkg.Redact(output),
	})
	return output, nil
}

// buildTaskResultReply creates a structured summary of what the agent did.
func buildTaskResultReply(summary string, changedFiles []string, verificationStatus, verificationOutput string) string {
	var b strings.Builder
	if summary != "" {
		b.WriteString("## Summary\n")
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	if len(changedFiles) > 0 {
		b.WriteString("## Changed files\n")
		for _, f := range changedFiles {
			b.WriteString("- `")
			b.WriteString(f)
			b.WriteString("`\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Verification\n")
	b.WriteString("Status: ")
	b.WriteString(verificationStatus)
	b.WriteString("\n")
	if verificationOutput != "" {
		b.WriteString(verificationOutput)
	}
	return b.String()
}

func finishAfterSuccessfulVerification(messages []agentruntime.Message, content string) (string, bool) {
	if len(messages) == 0 || strings.TrimSpace(content) == "" {
		return "", false
	}
	last := messages[len(messages)-1].Content
	if !strings.HasPrefix(last, "Tool observation:\n") {
		return "", false
	}
	var observation agentruntime.Observation
	if json.Unmarshal([]byte(strings.TrimPrefix(last, "Tool observation:\n")), &observation) != nil ||
		observation.Tool != "command" || observation.Status != "ok" {
		return "", false
	}
	return strings.TrimSpace(content), true
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid task ID", nil)
		return
	}
	s.runsMu.Lock()
	cancel, ok := s.activeRuns[id]
	if ok {
		delete(s.activeRuns, id)
	}
	s.runsMu.Unlock()
	if !ok {
		if task, err := s.tasks.GetByID(r.Context(), id); err == nil {
			if task.Status == "cancelled" {
				respondJSON(w, 200, map[string]string{"status": "cancelled"})
				return
			}
			if task.Status == "blocked" || task.Status == "failed" {
				if err := s.tasks.SetStatus(r.Context(), id, []string{task.Status}, "cancelled"); err != nil {
					respondError(w, 409, "TASK_STATE_CONFLICT", "task status changed before it could be cancelled", nil)
					return
				}
				s.broadcastTaskEvent("cancelled", id, task.Title)
				respondJSON(w, 200, map[string]string{"status": "cancelled"})
				return
			}
		}
		respondError(w, 409, "NOT_RUNNING", "task has no active execution", nil)
		return
	}
	cancel()
	respondJSON(w, 202, map[string]string{"status": "cancelling"})
}
