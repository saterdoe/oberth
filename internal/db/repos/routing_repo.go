package repos

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db"
)

// RoutingRule represents a single routing rule that matches requests to providers.
type RoutingRule struct {
	ID               uuid.UUID       `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Priority         int             `json:"priority"`
	IsActive         bool            `json:"is_active"`
	MatchRepoPattern *string         `json:"match_repo_pattern,omitempty"`
	MatchTaskType    *string         `json:"match_task_type,omitempty"`
	MatchUserID      *uuid.UUID      `json:"match_user_id,omitempty"`
	ProviderID       uuid.UUID       `json:"provider_id"`
	Model            string          `json:"model"`
	ExecutionGraph   json.RawMessage `json:"execution_graph,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// RoutingRuleRepo implements CRUD operations for the routing_rules table.
type RoutingRuleRepo struct {
	pool *pgxpool.Pool
}

// NewRoutingRuleRepo creates a new RoutingRuleRepo.
func NewRoutingRuleRepo(pool *pgxpool.Pool) *RoutingRuleRepo {
	return &RoutingRuleRepo{pool: pool}
}

// List returns all routing rules ordered by priority.
func (r *RoutingRuleRepo) List(ctx context.Context) ([]RoutingRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, priority, is_active,
		       match_repo_pattern, match_task_type, match_user_id,
		       provider_id, model, execution_graph,
		       created_at, updated_at
		FROM routing_rules
		ORDER BY priority ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RoutingRule
	for rows.Next() {
		var rule RoutingRule
		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Description, &rule.Priority, &rule.IsActive,
			&rule.MatchRepoPattern, &rule.MatchTaskType, &rule.MatchUserID,
			&rule.ProviderID, &rule.Model, &rule.ExecutionGraph,
			&rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// GetByID returns a routing rule by its ID. Returns db.ErrNotFound if no row exists.
func (r *RoutingRuleRepo) GetByID(ctx context.Context, id uuid.UUID) (*RoutingRule, error) {
	var rule RoutingRule
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, priority, is_active,
		       match_repo_pattern, match_task_type, match_user_id,
		       provider_id, model, execution_graph,
		       created_at, updated_at
		FROM routing_rules
		WHERE id = @id
	`, pgx.NamedArgs{"id": id}).Scan(
		&rule.ID, &rule.Name, &rule.Description, &rule.Priority, &rule.IsActive,
		&rule.MatchRepoPattern, &rule.MatchTaskType, &rule.MatchUserID,
		&rule.ProviderID, &rule.Model, &rule.ExecutionGraph,
		&rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return &rule, nil
}

// Create inserts a new routing rule. ID and timestamps are set by the database.
func (r *RoutingRuleRepo) Create(ctx context.Context, rule *RoutingRule) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO routing_rules (name, description, priority, is_active,
		                            match_repo_pattern, match_task_type, match_user_id,
		                            provider_id, model, execution_graph)
		VALUES (@name, @description, @priority, @is_active,
		        @match_repo_pattern, @match_task_type, @match_user_id,
		        @provider_id, @model, @execution_graph)
		RETURNING id, created_at, updated_at
	`, pgx.NamedArgs{
		"name":               rule.Name,
		"description":        rule.Description,
		"priority":           rule.Priority,
		"is_active":          rule.IsActive,
		"match_repo_pattern": rule.MatchRepoPattern,
		"match_task_type":    rule.MatchTaskType,
		"match_user_id":      rule.MatchUserID,
		"provider_id":        rule.ProviderID,
		"model":              rule.Model,
		"execution_graph":    rule.ExecutionGraph,
	}).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
}

// Update modifies an existing routing rule. Returns db.ErrNotFound if the row does not exist.
func (r *RoutingRuleRepo) Update(ctx context.Context, rule *RoutingRule) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE routing_rules
		SET name = @name,
		    description = @description,
		    priority = @priority,
		    is_active = @is_active,
		    match_repo_pattern = @match_repo_pattern,
		    match_task_type = @match_task_type,
		    match_user_id = @match_user_id,
		    provider_id = @provider_id,
		    model = @model,
		    execution_graph = @execution_graph,
		    updated_at = NOW()
		WHERE id = @id
	`, pgx.NamedArgs{
		"id":                 rule.ID,
		"name":               rule.Name,
		"description":        rule.Description,
		"priority":           rule.Priority,
		"is_active":          rule.IsActive,
		"match_repo_pattern": rule.MatchRepoPattern,
		"match_task_type":    rule.MatchTaskType,
		"match_user_id":      rule.MatchUserID,
		"provider_id":        rule.ProviderID,
		"model":              rule.Model,
		"execution_graph":    rule.ExecutionGraph,
	})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// Delete removes a routing rule by ID. Returns db.ErrNotFound if no row was deleted.
func (r *RoutingRuleRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM routing_rules WHERE id = @id
	`, pgx.NamedArgs{"id": id})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// Reorder updates the priority of routing rules based on the order of IDs in the slice.
// The first element gets priority 1, second gets priority 2, and so on.
func (r *RoutingRuleRepo) Reorder(ctx context.Context, ids []uuid.UUID) error {
	for i, id := range ids {
		_, err := r.pool.Exec(ctx, `
			UPDATE routing_rules
			SET priority = @priority, updated_at = NOW()
			WHERE id = @id
		`, pgx.NamedArgs{
			"id":       id,
			"priority": i + 1,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
