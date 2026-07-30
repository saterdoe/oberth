package repos

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db"
)

// Provider represents a single LLM provider configuration.
type Provider struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	ProviderType    string    `json:"provider_type"`
	BaseURL         *string   `json:"base_url,omitempty"`
	APIKeyEncrypted *string   `json:"-"`
	DefaultModel    string    `json:"default_model"`
	Models          string    `json:"models"`
	IsActive        bool      `json:"is_active"`
	Priority        int       `json:"priority"`
	RateLimitRPM    *int      `json:"rate_limit_rpm,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ProviderRepo implements CRUD operations for the providers table.
type ProviderRepo struct {
	pool *pgxpool.Pool
}

// NewProviderRepo creates a new ProviderRepo.
func NewProviderRepo(pool *pgxpool.Pool) *ProviderRepo {
	return &ProviderRepo{pool: pool}
}

// List returns all providers ordered by priority.
func (r *ProviderRepo) List(ctx context.Context) ([]Provider, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, provider_type, base_url, api_key_encrypted,
		       default_model, models, is_active, priority, rate_limit_rpm,
		       created_at, updated_at
		FROM providers
		ORDER BY priority ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []Provider
	for rows.Next() {
		var p Provider
		if err := rows.Scan(
			&p.ID, &p.Name, &p.ProviderType, &p.BaseURL, &p.APIKeyEncrypted,
			&p.DefaultModel, &p.Models, &p.IsActive, &p.Priority, &p.RateLimitRPM,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// GetByID returns a provider by its ID. Returns db.ErrNotFound if no row exists.
func (r *ProviderRepo) GetByID(ctx context.Context, id uuid.UUID) (*Provider, error) {
	var p Provider
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, provider_type, base_url, api_key_encrypted,
		       default_model, models, is_active, priority, rate_limit_rpm,
		       created_at, updated_at
		FROM providers
		WHERE id = @id
	`, pgx.NamedArgs{"id": id}).Scan(
		&p.ID, &p.Name, &p.ProviderType, &p.BaseURL, &p.APIKeyEncrypted,
		&p.DefaultModel, &p.Models, &p.IsActive, &p.Priority, &p.RateLimitRPM,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// Create inserts a new provider. The ID and timestamps are set by the database.
func (r *ProviderRepo) Create(ctx context.Context, p *Provider) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO providers (name, provider_type, base_url, api_key_encrypted,
		                       default_model, models, is_active, priority, rate_limit_rpm)
		VALUES (@name, @provider_type, @base_url, @api_key_encrypted,
		        @default_model, @models, @is_active, @priority, @rate_limit_rpm)
		RETURNING id, created_at, updated_at
	`, pgx.NamedArgs{
		"name":              p.Name,
		"provider_type":     p.ProviderType,
		"base_url":          p.BaseURL,
		"api_key_encrypted": p.APIKeyEncrypted,
		"default_model":     p.DefaultModel,
		"models":            p.Models,
		"is_active":         p.IsActive,
		"priority":          p.Priority,
		"rate_limit_rpm":    p.RateLimitRPM,
	}).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// Update modifies an existing provider. Returns db.ErrNotFound if the row does not exist.
func (r *ProviderRepo) Update(ctx context.Context, p *Provider) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE providers
		SET name = @name,
		    provider_type = @provider_type,
		    base_url = @base_url,
		    api_key_encrypted = @api_key_encrypted,
		    default_model = @default_model,
		    models = @models,
		    is_active = @is_active,
		    priority = @priority,
		    rate_limit_rpm = @rate_limit_rpm,
		    updated_at = NOW()
		WHERE id = @id
	`, pgx.NamedArgs{
		"id":                p.ID,
		"name":              p.Name,
		"provider_type":     p.ProviderType,
		"base_url":          p.BaseURL,
		"api_key_encrypted": p.APIKeyEncrypted,
		"default_model":     p.DefaultModel,
		"models":            p.Models,
		"is_active":         p.IsActive,
		"priority":          p.Priority,
		"rate_limit_rpm":    p.RateLimitRPM,
	})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}

// Delete removes a provider by ID. Returns db.ErrNotFound if no row was deleted.
func (r *ProviderRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM providers WHERE id = @id
	`, pgx.NamedArgs{"id": id})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return db.ErrNotFound
	}
	return nil
}
