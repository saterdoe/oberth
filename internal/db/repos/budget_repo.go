package repos

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db"
)

var ErrBudgetReservationExceeded = errors.New("budget reservation exceeds hard limit")

type BudgetReservation struct {
	ID        uuid.UUID
	BudgetIDs []uuid.UUID
	Amount    float64
	ExpiresAt time.Time
}

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

func (r *BudgetRepo) Reserve(ctx context.Context, providerID *uuid.UUID, amount float64, ttl time.Duration) (*BudgetReservation, error) {
	if amount <= 0 {
		return &BudgetReservation{}, nil
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id, hard_limit, current_spend FROM budgets
		WHERE is_active AND (provider_id IS NULL OR provider_id = @provider_id) ORDER BY id FOR UPDATE`, pgx.NamedArgs{"provider_id": providerID})
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id          uuid.UUID
		hard, spent float64
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.hard, &c.spent); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	reservation := &BudgetReservation{ID: uuid.New(), Amount: amount, ExpiresAt: time.Now().Add(ttl)}
	for _, c := range candidates {
		var active float64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0) FROM cost_reservations WHERE budget_id=$1 AND state='active' AND expires_at>NOW()`, c.id).Scan(&active); err != nil {
			return nil, err
		}
		if c.hard > 0 && c.spent+active+amount > c.hard {
			return nil, ErrBudgetReservationExceeded
		}
		reservation.BudgetIDs = append(reservation.BudgetIDs, c.id)
	}
	for _, id := range reservation.BudgetIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO cost_reservations(reservation_id,budget_id,amount,expires_at) VALUES($1,$2,$3,$4)`, reservation.ID, id, amount, reservation.ExpiresAt); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return reservation, nil
}

func (r *BudgetRepo) SetReservationState(ctx context.Context, id uuid.UUID, state string) error {
	if state != "committed" && state != "released" {
		return errors.New("invalid reservation terminal state")
	}
	_, err := r.pool.Exec(ctx, `UPDATE cost_reservations SET state=$2, updated_at=NOW() WHERE reservation_id=$1 AND state='active'`, id, state)
	return err
}

func (r *BudgetRepo) ReconcileExpiredReservations(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE cost_reservations SET state='expired', updated_at=NOW() WHERE state='active' AND expires_at<=NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
