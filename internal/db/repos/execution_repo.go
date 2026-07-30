package repos

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db"
)

// ExecutionLog represents a single execution step within a session.
type ExecutionLog struct {
	ID           uuid.UUID  `json:"id"`
	SessionID    uuid.UUID  `json:"session_id"`
	StepID       string     `json:"step_id"`
	ProviderID   *uuid.UUID `json:"provider_id,omitempty"`
	Model        string     `json:"model"`
	ParentStep   *string    `json:"parent_step,omitempty"`
	Status       string     `json:"status"`
	TokensInput  int        `json:"tokens_input"`
	TokensOutput int        `json:"tokens_output"`
	Cost         float64    `json:"cost"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// ExecutionLogRepo implements operations for the execution_logs table.
type ExecutionLogRepo struct {
	pool *pgxpool.Pool
}

// NewExecutionLogRepo creates a new ExecutionLogRepo.
func NewExecutionLogRepo(pool *pgxpool.Pool) *ExecutionLogRepo {
	return &ExecutionLogRepo{pool: pool}
}

// Create inserts a new execution log entry. ID is set by the database.
func (r *ExecutionLogRepo) Create(ctx context.Context, e *ExecutionLog) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO execution_logs (session_id, step_id, provider_id, model,
		                             parent_step, status, tokens_input, tokens_output,
		                             cost, error_message, started_at, completed_at)
		VALUES (@session_id, @step_id, @provider_id, @model,
		        @parent_step, @status, @tokens_input, @tokens_output,
		        @cost, @error_message, @started_at, @completed_at)
		RETURNING id
	`, pgx.NamedArgs{
		"session_id":    e.SessionID,
		"step_id":       e.StepID,
		"provider_id":   e.ProviderID,
		"model":         e.Model,
		"parent_step":   e.ParentStep,
		"status":        e.Status,
		"tokens_input":  e.TokensInput,
		"tokens_output": e.TokensOutput,
		"cost":          e.Cost,
		"error_message": e.ErrorMessage,
		"started_at":    e.StartedAt,
		"completed_at":  e.CompletedAt,
	}).Scan(&e.ID)
}

// UpdateStatus updates the status and optionally the error_message of an execution log.
// Sets completed_at if the new status is terminal (success, failed, or skipped).
// Returns db.ErrNotFound if the row does not exist.
func (r *ExecutionLogRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string, errMsg *string) error {
	terminal := status == "success" || status == "failed" || status == "skipped"

	var query string
	if terminal {
		query = `
			UPDATE execution_logs
			SET status = @status,
			    error_message = @error_message,
			    completed_at = NOW()
			WHERE id = @id
		`
	} else {
		query = `
			UPDATE execution_logs
			SET status = @status,
			    error_message = @error_message
			WHERE id = @id
		`
	}

	tag, err := r.pool.Exec(ctx, query, pgx.NamedArgs{
		"id":            id,
		"status":        status,
		"error_message": errMsg,
	})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// ListBySession returns all execution logs for a given session, ordered by step.
func (r *ExecutionLogRepo) ListBySession(ctx context.Context, sessionID uuid.UUID) ([]ExecutionLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, session_id, step_id, provider_id, model,
		       parent_step, status, tokens_input, tokens_output,
		       cost, error_message, started_at, completed_at
		FROM execution_logs
		WHERE session_id = @session_id
		ORDER BY step_id ASC
	`, pgx.NamedArgs{"session_id": sessionID})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ExecutionLog
	for rows.Next() {
		var el ExecutionLog
		if err := rows.Scan(
			&el.ID, &el.SessionID, &el.StepID, &el.ProviderID, &el.Model,
			&el.ParentStep, &el.Status, &el.TokensInput, &el.TokensOutput,
			&el.Cost, &el.ErrorMessage, &el.StartedAt, &el.CompletedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, el)
	}
	return logs, rows.Err()
}
