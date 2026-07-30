package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db"
)

// ContextSource tracks what context was used for a task.
type ContextSource struct {
	SourceType   string  `json:"source_type"`
	SourcePath   string  `json:"source_path"`
	Relevance    float64 `json:"relevance"`
	TokensApprox int     `json:"tokens_approx"`
	SentToCloud  bool    `json:"sent_to_cloud"`
	Excluded     bool    `json:"excluded"`
	Reason       string  `json:"reason,omitempty"`
}

// ApprovalGate defines a rule for requiring approval.
type ApprovalGate struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description,omitempty"`
	RepoPattern     *string    `json:"repo_pattern,omitempty"`
	TaskType        *string    `json:"task_type,omitempty"`
	ProviderID      *uuid.UUID `json:"provider_id,omitempty"`
	RequireApproval bool       `json:"require_approval"`
	RequireReview   bool       `json:"require_review"`
	DenyCloud       bool       `json:"deny_cloud"`
	RequireTests    bool       `json:"require_tests"`
	MaxCost         *float64   `json:"max_cost,omitempty"`
	Priority        int        `json:"priority"`
	IsActive        bool       `json:"is_active"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Session represents a single task/session with full workflow state.
type Session struct {
	ID               uuid.UUID       `json:"id"`
	TaskID           *uuid.UUID      `json:"task_id,omitempty"`
	UserID           *uuid.UUID      `json:"user_id,omitempty"`
	RepoPath         *string         `json:"repo_path,omitempty"`
	Branch           *string         `json:"branch,omitempty"`
	TaskType         string          `json:"task_type"`
	TaskDescription  *string         `json:"task_description,omitempty"`
	Plan             json.RawMessage `json:"plan,omitempty"`
	DiffSummary      *string         `json:"diff_summary,omitempty"`
	Artifacts        json.RawMessage `json:"artifacts,omitempty"`
	ContextUsed      json.RawMessage `json:"context_used,omitempty"`
	ProviderID       *uuid.UUID      `json:"provider_id,omitempty"`
	Model            *string         `json:"model,omitempty"`
	TokensInput      int             `json:"tokens_input"`
	TokensOutput     int             `json:"tokens_output"`
	Cost             float64         `json:"cost"`
	DurationMs       *int            `json:"duration_ms,omitempty"`
	Status           string          `json:"status"`
	ApprovalRequired bool            `json:"approval_required"`
	RiskScore        *string         `json:"risk_score,omitempty"`
	ApprovedBy       *uuid.UUID      `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time      `json:"approved_at,omitempty"`
	StartedAt        time.Time       `json:"started_at"`
	EndedAt          *time.Time      `json:"ended_at,omitempty"`
}

// SessionFilter defines filters for listing sessions/tasks.
type SessionFilter struct {
	Status *string
	UserID *uuid.UUID
	Since  *time.Time
	Until  *time.Time
	Offset int
	Limit  int
}

// SessionRepo implements CRUD operations for the sessions table.
type SessionRepo struct {
	pool *pgxpool.Pool
}

func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo {
	return &SessionRepo{pool: pool}
}

func (r *SessionRepo) Create(ctx context.Context, s *Session) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	planJSON := nullJSON(s.Plan)
	artifactsJSON := nullJSON(s.Artifacts)
	ctxJSON := nullJSON(s.ContextUsed)
	return r.pool.QueryRow(ctx, `
		INSERT INTO sessions (id, task_id, user_id, repo_path, branch, task_type, task_description,
		                      plan, diff_summary, artifacts, context_used,
		                      provider_id, model, tokens_input, tokens_output,
		                      cost, duration_ms, status, approval_required, risk_score)
		VALUES (@id, @task_id, @user_id, @repo_path, @branch, @task_type, @task_description,
		        @plan, @diff_summary, @artifacts, @context_used,
		        @provider_id, @model, @tokens_input, @tokens_output,
		        @cost, @duration_ms, @status, @approval_required, @risk_score)
		RETURNING id, started_at
	`, pgx.NamedArgs{
		"id":                s.ID,
		"task_id":           s.TaskID,
		"user_id":           s.UserID,
		"repo_path":         s.RepoPath,
		"branch":            s.Branch,
		"task_type":         s.TaskType,
		"task_description":  s.TaskDescription,
		"plan":              planJSON,
		"diff_summary":      s.DiffSummary,
		"artifacts":         artifactsJSON,
		"context_used":      ctxJSON,
		"provider_id":       s.ProviderID,
		"model":             s.Model,
		"tokens_input":      s.TokensInput,
		"tokens_output":     s.TokensOutput,
		"cost":              s.Cost,
		"duration_ms":       s.DurationMs,
		"status":            s.Status,
		"approval_required": s.ApprovalRequired,
		"risk_score":        s.RiskScore,
	}).Scan(&s.ID, &s.StartedAt)
}

func (r *SessionRepo) GetByID(ctx context.Context, id uuid.UUID) (*Session, error) {
	var s Session
	err := r.pool.QueryRow(ctx, `
		SELECT id, task_id, user_id, repo_path, branch, task_type, task_description,
		       plan, diff_summary, artifacts, context_used,
		       provider_id, model, tokens_input, tokens_output,
		       cost, duration_ms, status, approval_required, risk_score,
		       approved_by, approved_at, started_at, ended_at
		FROM sessions
		WHERE id = @id
	`, pgx.NamedArgs{"id": id}).Scan(
		&s.ID, &s.TaskID, &s.UserID, &s.RepoPath, &s.Branch, &s.TaskType, &s.TaskDescription,
		&s.Plan, &s.DiffSummary, &s.Artifacts, &s.ContextUsed,
		&s.ProviderID, &s.Model, &s.TokensInput, &s.TokensOutput,
		&s.Cost, &s.DurationMs, &s.Status, &s.ApprovalRequired, &s.RiskScore,
		&s.ApprovedBy, &s.ApprovedAt, &s.StartedAt, &s.EndedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepo) List(ctx context.Context, filter SessionFilter) ([]Session, error) {
	query := `SELECT id, task_id, user_id, repo_path, branch, task_type, task_description,
	                 plan, diff_summary, artifacts, context_used,
	                 provider_id, model, tokens_input, tokens_output,
	                 cost, duration_ms, status, approval_required, risk_score,
	                 approved_by, approved_at, started_at, ended_at
	          FROM sessions WHERE 1=1`
	args := make(map[string]interface{})

	if filter.Status != nil {
		query += ` AND status = @status`
		args["status"] = *filter.Status
	}
	if filter.UserID != nil {
		query += ` AND user_id = @user_id`
		args["user_id"] = *filter.UserID
	}
	if filter.Since != nil {
		query += ` AND started_at >= @since`
		args["since"] = *filter.Since
	}
	if filter.Until != nil {
		query += ` AND started_at < @until`
		args["until"] = *filter.Until
	}

	query += ` ORDER BY started_at DESC`

	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, filter.Limit)
	} else {
		query += ` LIMIT 100`
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, filter.Offset)
	}

	rows, err := r.pool.Query(ctx, query, pgx.NamedArgs(args))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(
			&s.ID, &s.TaskID, &s.UserID, &s.RepoPath, &s.Branch, &s.TaskType, &s.TaskDescription,
			&s.Plan, &s.DiffSummary, &s.Artifacts, &s.ContextUsed,
			&s.ProviderID, &s.Model, &s.TokensInput, &s.TokensOutput,
			&s.Cost, &s.DurationMs, &s.Status, &s.ApprovalRequired, &s.RiskScore,
			&s.ApprovedBy, &s.ApprovedAt, &s.StartedAt, &s.EndedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func (r *SessionRepo) Update(ctx context.Context, s *Session) error {
	var sets []string
	args := pgx.NamedArgs{"id": s.ID}

	if s.UserID != nil {
		sets = append(sets, "user_id = @user_id")
		args["user_id"] = *s.UserID
	}
	if s.RepoPath != nil {
		sets = append(sets, "repo_path = @repo_path")
		args["repo_path"] = *s.RepoPath
	}
	if s.Branch != nil {
		sets = append(sets, "branch = @branch")
		args["branch"] = *s.Branch
	}
	sets = append(sets, "task_type = @task_type")
	args["task_type"] = s.TaskType
	if s.TaskDescription != nil {
		sets = append(sets, "task_description = @task_description")
		args["task_description"] = *s.TaskDescription
	}
	if s.Plan != nil {
		sets = append(sets, "plan = @plan")
		args["plan"] = nullJSON(s.Plan)
	}
	if s.DiffSummary != nil {
		sets = append(sets, "diff_summary = @diff_summary")
		args["diff_summary"] = *s.DiffSummary
	}
	if s.Artifacts != nil {
		sets = append(sets, "artifacts = @artifacts")
		args["artifacts"] = nullJSON(s.Artifacts)
	}
	if s.ContextUsed != nil {
		sets = append(sets, "context_used = @context_used")
		args["context_used"] = nullJSON(s.ContextUsed)
	}
	if s.ProviderID != nil {
		sets = append(sets, "provider_id = @provider_id")
		args["provider_id"] = *s.ProviderID
	}
	if s.Model != nil {
		sets = append(sets, "model = @model")
		args["model"] = *s.Model
	}
	sets = append(sets, "tokens_input = @tokens_input")
	args["tokens_input"] = s.TokensInput
	sets = append(sets, "tokens_output = @tokens_output")
	args["tokens_output"] = s.TokensOutput
	sets = append(sets, "cost = @cost")
	args["cost"] = s.Cost
	if s.DurationMs != nil {
		sets = append(sets, "duration_ms = @duration_ms")
		args["duration_ms"] = *s.DurationMs
	}
	sets = append(sets, "status = @status")
	args["status"] = s.Status
	sets = append(sets, "approval_required = @approval_required")
	args["approval_required"] = s.ApprovalRequired
	if s.RiskScore != nil {
		sets = append(sets, "risk_score = @risk_score")
		args["risk_score"] = *s.RiskScore
	}
	if s.ApprovedBy != nil {
		sets = append(sets, "approved_by = @approved_by")
		args["approved_by"] = *s.ApprovedBy
	}
	if s.ApprovedAt != nil {
		sets = append(sets, "approved_at = @approved_at")
		args["approved_at"] = *s.ApprovedAt
	}
	if s.EndedAt != nil {
		sets = append(sets, "ended_at = @ended_at")
		args["ended_at"] = *s.EndedAt
	}

	if len(sets) == 0 {
		return nil
	}

	query := fmt.Sprintf(`UPDATE sessions SET %s, updated_at = NOW() WHERE id = @id`,
		strings.Join(sets, ", "))

	tag, err := r.pool.Exec(ctx, query, args)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func (r *SessionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE id = @id`,
		pgx.NamedArgs{"id": id})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

func nullJSON(data json.RawMessage) interface{} {
	if data == nil || string(data) == "null" {
		return nil
	}
	return data
}
