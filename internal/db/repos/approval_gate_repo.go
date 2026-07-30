package repos

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ApprovalGateRepo implements CRUD for the approval_gates table.
type ApprovalGateRepo struct {
	pool *pgxpool.Pool
}

func NewApprovalGateRepo(pool *pgxpool.Pool) *ApprovalGateRepo {
	return &ApprovalGateRepo{pool: pool}
}

func (r *ApprovalGateRepo) Create(ctx context.Context, g *ApprovalGate) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO approval_gates (name, description, repo_pattern, task_type, provider_id,
		                            require_approval, require_review, deny_cloud, require_tests,
		                            max_cost, priority, is_active)
		VALUES (@name, @description, @repo_pattern, @task_type, @provider_id,
		        @require_approval, @require_review, @deny_cloud, @require_tests,
		        @max_cost, @priority, @is_active)
		RETURNING id, created_at, updated_at
	`, pgx.NamedArgs{
		"name":             g.Name,
		"description":      g.Description,
		"repo_pattern":     g.RepoPattern,
		"task_type":        g.TaskType,
		"provider_id":      g.ProviderID,
		"require_approval": g.RequireApproval,
		"require_review":   g.RequireReview,
		"deny_cloud":       g.DenyCloud,
		"require_tests":    g.RequireTests,
		"max_cost":         g.MaxCost,
		"priority":         g.Priority,
		"is_active":        g.IsActive,
	}).Scan(&g.ID, &g.CreatedAt, &g.UpdatedAt)
}

func (r *ApprovalGateRepo) List(ctx context.Context) ([]ApprovalGate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, repo_pattern, task_type, provider_id,
		       require_approval, require_review, deny_cloud, require_tests,
		       max_cost, priority, is_active, created_at, updated_at
		FROM approval_gates
		ORDER BY priority DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var gates []ApprovalGate
	for rows.Next() {
		var g ApprovalGate
		if err := rows.Scan(
			&g.ID, &g.Name, &g.Description, &g.RepoPattern, &g.TaskType, &g.ProviderID,
			&g.RequireApproval, &g.RequireReview, &g.DenyCloud, &g.RequireTests,
			&g.MaxCost, &g.Priority, &g.IsActive, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		gates = append(gates, g)
	}
	return gates, rows.Err()
}

func (r *ApprovalGateRepo) Match(ctx context.Context, repoPath, taskType string) (*ApprovalGate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, repo_pattern, task_type, provider_id,
		       require_approval, require_review, deny_cloud, require_tests,
		       max_cost, priority, is_active, created_at, updated_at
		FROM approval_gates
		WHERE is_active = true
		ORDER BY priority DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var g ApprovalGate
		if err := rows.Scan(
			&g.ID, &g.Name, &g.Description, &g.RepoPattern, &g.TaskType, &g.ProviderID,
			&g.RequireApproval, &g.RequireReview, &g.DenyCloud, &g.RequireTests,
			&g.MaxCost, &g.Priority, &g.IsActive, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if g.RepoPattern != nil && !matchGlob(*g.RepoPattern, repoPath) {
			continue
		}
		if g.TaskType != nil && *g.TaskType != taskType {
			continue
		}
		return &g, nil
	}
	return nil, nil
}

func (r *ApprovalGateRepo) Update(ctx context.Context, id uuid.UUID, g *ApprovalGate) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE approval_gates SET
			name = @name, description = @description, repo_pattern = @repo_pattern,
			task_type = @task_type, provider_id = @provider_id,
			require_approval = @require_approval, require_review = @require_review,
			deny_cloud = @deny_cloud, require_tests = @require_tests,
			max_cost = @max_cost, priority = @priority, is_active = @is_active,
			updated_at = NOW()
		WHERE id = @id
	`, pgx.NamedArgs{
		"id":               id,
		"name":             g.Name,
		"description":      g.Description,
		"repo_pattern":     g.RepoPattern,
		"task_type":        g.TaskType,
		"provider_id":      g.ProviderID,
		"require_approval": g.RequireApproval,
		"require_review":   g.RequireReview,
		"deny_cloud":       g.DenyCloud,
		"require_tests":    g.RequireTests,
		"max_cost":         g.MaxCost,
		"priority":         g.Priority,
		"is_active":        g.IsActive,
	})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("approval gate not found")
	}
	return nil
}

func (r *ApprovalGateRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM approval_gates WHERE id = @id`,
		pgx.NamedArgs{"id": id})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("approval gate not found")
	}
	return nil
}

func matchGlob(pattern, path string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	// Simple prefix/suffix matching
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		mid := strings.TrimPrefix(strings.TrimSuffix(pattern, "*"), "*")
		return strings.Contains(path, mid)
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(path, strings.TrimPrefix(pattern, "*"))
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(pattern, "*"))
	}
	return path == pattern
}
