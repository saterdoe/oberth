package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogEntry represents a single audit log record.
type AuditLogEntry struct {
	ID        uuid.UUID       `json:"id"`
	SessionID *uuid.UUID      `json:"session_id,omitempty"`
	Action    string          `json:"action"`
	Actor     string          `json:"actor"`
	Details   json.RawMessage `json:"details"`
	CreatedAt time.Time       `json:"created_at"`
}

// AuditFilter defines filters for listing audit log entries.
type AuditFilter struct {
	Action *string
	Actor  *string
	Since  *time.Time
	Until  *time.Time
	Offset int
	Limit  int
}

// AuditRepo implements operations for the audit_log table.
type AuditRepo struct {
	pool *pgxpool.Pool
}

// NewAuditRepo creates a new AuditRepo.
func NewAuditRepo(pool *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{pool: pool}
}

// Create inserts a new audit log entry. ID and CreatedAt are set by the database.
func (r *AuditRepo) Create(ctx context.Context, a *AuditLogEntry) error {
	if a.Details == nil {
		a.Details = json.RawMessage(`{}`)
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO audit_log (session_id, action, actor, details)
		VALUES (@session_id, @action, @actor, @details)
		RETURNING id, created_at
	`, pgx.NamedArgs{
		"session_id": a.SessionID,
		"action":     a.Action,
		"actor":      a.Actor,
		"details":    a.Details,
	}).Scan(&a.ID, &a.CreatedAt)
}

// List returns audit log entries matching the provided filter, paginated.
func (r *AuditRepo) List(ctx context.Context, filter AuditFilter) ([]AuditLogEntry, error) {
	query := `SELECT id, session_id, action, actor, details, created_at
	          FROM audit_log WHERE 1=1`
	args := make(map[string]interface{})

	if filter.Action != nil {
		query += ` AND action = @action`
		args["action"] = *filter.Action
	}
	if filter.Actor != nil {
		query += ` AND actor = @actor`
		args["actor"] = *filter.Actor
	}
	if filter.Since != nil {
		query += ` AND created_at >= @since`
		args["since"] = *filter.Since
	}
	if filter.Until != nil {
		query += ` AND created_at < @until`
		args["until"] = *filter.Until
	}

	query += ` ORDER BY created_at DESC`

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

	var entries []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		if err := rows.Scan(
			&e.ID, &e.SessionID, &e.Action, &e.Actor, &e.Details, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
