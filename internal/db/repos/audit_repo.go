package repos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogEntry represents a single audit log record.
type AuditLogEntry struct {
	ID            uuid.UUID       `json:"id"`
	SessionID     *uuid.UUID      `json:"session_id,omitempty"`
	Action        string          `json:"action"`
	Actor         string          `json:"actor"`
	Target        string          `json:"target"`
	Decision      string          `json:"decision"`
	CorrelationID uuid.UUID       `json:"correlation_id"`
	Sequence      int64           `json:"sequence"`
	PrevHash      string          `json:"prev_hash"`
	EntryHash     string          `json:"entry_hash"`
	Details       json.RawMessage `json:"details"`
	CreatedAt     time.Time       `json:"created_at"`
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
	if a.CorrelationID == uuid.Nil {
		a.CorrelationID = uuid.New()
	}
	if a.Target == "" {
		a.Target = "unspecified"
	}
	if a.Decision == "" {
		a.Decision = "recorded"
	}
	return r.pool.QueryRow(ctx, `
		WITH chain_lock AS (
			SELECT pg_advisory_xact_lock(hashtextextended('oberth:audit-chain',0))
		), previous AS (
			SELECT COALESCE(MAX(sequence),0) AS sequence,
			       COALESCE((SELECT entry_hash FROM audit_log ORDER BY sequence DESC LIMIT 1),'') AS hash
			FROM audit_log,chain_lock
		), inserted AS (
			INSERT INTO audit_log (session_id,action,actor,target,decision,correlation_id,details,sequence,prev_hash,entry_hash)
			SELECT @session_id::uuid,@action::text,@actor::text,@target::text,@decision::text,@correlation_id::uuid,@details::jsonb,
			       previous.sequence+1,previous.hash,
			       encode(digest(concat_ws('|',previous.hash,COALESCE(@session_id::uuid::text,''),@correlation_id::uuid::text,
			           @action::text,@actor::text,@target::text,@decision::text,@details::jsonb::text),'sha256'),'hex')
			FROM previous
			RETURNING id,created_at,sequence,prev_hash,entry_hash
		)
		SELECT id,created_at,sequence,prev_hash,entry_hash FROM inserted
	`, pgx.NamedArgs{
		"session_id":     a.SessionID,
		"action":         a.Action,
		"actor":          a.Actor,
		"target":         a.Target,
		"decision":       a.Decision,
		"correlation_id": a.CorrelationID,
		"details":        a.Details,
	}).Scan(&a.ID, &a.CreatedAt, &a.Sequence, &a.PrevHash, &a.EntryHash)
}

func (r *AuditRepo) VerifyChain(ctx context.Context) error {
	var sequence int64
	err := r.pool.QueryRow(ctx, `
		WITH verified AS (
			SELECT sequence,prev_hash,entry_hash,
			       COALESCE(LAG(entry_hash) OVER (ORDER BY sequence),'') AS expected_prev,
			       encode(digest(concat_ws('|',prev_hash,COALESCE(session_id::text,''),correlation_id::text,
			           action,actor,target,decision,details::text),'sha256'),'hex') AS expected_hash
			FROM audit_log
		)
		SELECT sequence FROM verified
		WHERE prev_hash<>expected_prev OR entry_hash<>expected_hash
		ORDER BY sequence LIMIT 1`).Scan(&sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("audit chain integrity failure at sequence %d", sequence)
}

// List returns audit log entries matching the provided filter, paginated.
func (r *AuditRepo) List(ctx context.Context, filter AuditFilter) ([]AuditLogEntry, error) {
	query := `SELECT id, session_id, action, actor, target, decision, correlation_id, sequence, prev_hash, entry_hash, details, created_at
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
			&e.ID, &e.SessionID, &e.Action, &e.Actor, &e.Target, &e.Decision, &e.CorrelationID,
			&e.Sequence, &e.PrevHash, &e.EntryHash, &e.Details, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
