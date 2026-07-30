package repos

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db"
)

// Budget represents a cost budget with soft and hard limits.
type Budget struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	ProviderID   *uuid.UUID `json:"provider_id,omitempty"`
	SoftLimit    float64    `json:"soft_limit"`
	HardLimit    float64    `json:"hard_limit"`
	Period       string     `json:"period"`
	CurrentSpend float64    `json:"current_spend"`
	PeriodStart  time.Time  `json:"period_start"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// BudgetRepo implements CRUD operations for the budgets table.
type BudgetRepo struct {
	pool *pgxpool.Pool
}

// NewBudgetRepo creates a new BudgetRepo.
func NewBudgetRepo(pool *pgxpool.Pool) *BudgetRepo {
	return &BudgetRepo{pool: pool}
}

// List returns all budgets ordered by name.
func (r *BudgetRepo) List(ctx context.Context) ([]Budget, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, provider_id,
		       soft_limit, hard_limit, period, current_spend,
		       period_start, is_active, created_at, updated_at
		FROM budgets
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var budgets []Budget
	for rows.Next() {
		var b Budget
		if err := rows.Scan(
			&b.ID, &b.Name, &b.Description, &b.ProviderID,
			&b.SoftLimit, &b.HardLimit, &b.Period, &b.CurrentSpend,
			&b.PeriodStart, &b.IsActive, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, err
		}
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}

// GetByID returns a budget by its ID. Returns db.ErrNotFound if no row exists.
func (r *BudgetRepo) GetByID(ctx context.Context, id uuid.UUID) (*Budget, error) {
	var b Budget
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, provider_id,
		       soft_limit, hard_limit, period, current_spend,
		       period_start, is_active, created_at, updated_at
		FROM budgets
		WHERE id = @id
	`, pgx.NamedArgs{"id": id}).Scan(
		&b.ID, &b.Name, &b.Description, &b.ProviderID,
		&b.SoftLimit, &b.HardLimit, &b.Period, &b.CurrentSpend,
		&b.PeriodStart, &b.IsActive, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return &b, nil
}

// Create inserts a new budget. ID and timestamps are set by the database.
func (r *BudgetRepo) Create(ctx context.Context, b *Budget) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO budgets (name, description, provider_id,
		                     soft_limit, hard_limit, period, current_spend,
		                     period_start, is_active)
		VALUES (@name, @description, @provider_id,
		        @soft_limit, @hard_limit, @period, @current_spend,
		        @period_start, @is_active)
		RETURNING id, created_at, updated_at
	`, pgx.NamedArgs{
		"name":          b.Name,
		"description":   b.Description,
		"provider_id":   b.ProviderID,
		"soft_limit":    b.SoftLimit,
		"hard_limit":    b.HardLimit,
		"period":        b.Period,
		"current_spend": b.CurrentSpend,
		"period_start":  b.PeriodStart,
		"is_active":     b.IsActive,
	}).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

// Update modifies an existing budget. Returns db.ErrNotFound if the row does not exist.
func (r *BudgetRepo) Update(ctx context.Context, b *Budget) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE budgets
		SET name = @name,
		    description = @description,
		    provider_id = @provider_id,
		    soft_limit = @soft_limit,
		    hard_limit = @hard_limit,
		    period = @period,
		    current_spend = @current_spend,
		    period_start = @period_start,
		    is_active = @is_active,
		    updated_at = NOW()
		WHERE id = @id
	`, pgx.NamedArgs{
		"id":            b.ID,
		"name":          b.Name,
		"description":   b.Description,
		"provider_id":   b.ProviderID,
		"soft_limit":    b.SoftLimit,
		"hard_limit":    b.HardLimit,
		"period":        b.Period,
		"current_spend": b.CurrentSpend,
		"period_start":  b.PeriodStart,
		"is_active":     b.IsActive,
	})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// Delete removes a budget by ID. Returns db.ErrNotFound if no row was deleted.
func (r *BudgetRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM budgets WHERE id = @id
	`, pgx.NamedArgs{"id": id})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// AddSpend atomically increments the current_spend of a budget by the given amount.
// Returns db.ErrNotFound if the budget does not exist.
func (r *BudgetRepo) AddSpend(ctx context.Context, id uuid.UUID, amount float64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE budgets
		SET current_spend = current_spend + @amount,
		    updated_at = NOW()
		WHERE id = @id
	`, pgx.NamedArgs{
		"id":     id,
		"amount": amount,
	})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}
