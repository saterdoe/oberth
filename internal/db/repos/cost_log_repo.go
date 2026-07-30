package repos

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CostLog represents a single cost log entry for an LLM API call.
type CostLog struct {
	ID           uuid.UUID  `json:"id"`
	SessionID    uuid.UUID  `json:"session_id"`
	ProviderID   *uuid.UUID `json:"provider_id,omitempty"`
	Model        string     `json:"model"`
	TokensInput  int        `json:"tokens_input"`
	TokensOutput int        `json:"tokens_output"`
	CostInput    float64    `json:"cost_input"`
	CostOutput   float64    `json:"cost_output"`
	CostTotal    float64    `json:"cost_total"`
	CacheHit     bool       `json:"cache_hit"`
	CreatedAt    time.Time  `json:"created_at"`
}

// CostLogFilter defines filters for listing cost logs.
type CostLogFilter struct {
	ProviderID *uuid.UUID
	Since      *time.Time
	Until      *time.Time
	Offset     int
	Limit      int
}

// CostSummary holds aggregated cost information.
type CostSummary struct {
	TotalCost   float64            `json:"total_cost"`
	TotalTokens int64              `json:"total_tokens"`
	ByProvider  map[string]float64 `json:"by_provider"`
}

// CostLogRepo implements operations for the cost_logs table.
type CostLogRepo struct {
	pool *pgxpool.Pool
}

// NewCostLogRepo creates a new CostLogRepo.
func NewCostLogRepo(pool *pgxpool.Pool) *CostLogRepo {
	return &CostLogRepo{pool: pool}
}

// Create inserts a new cost log entry. ID and CreatedAt are set by the database.
func (r *CostLogRepo) Create(ctx context.Context, c *CostLog) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO cost_logs (session_id, provider_id, model,
		                       tokens_input, tokens_output,
		                       cost_input, cost_output, cost_total, cache_hit)
		VALUES (@session_id, @provider_id, @model,
		        @tokens_input, @tokens_output,
		        @cost_input, @cost_output, @cost_total, @cache_hit)
		RETURNING id, created_at
	`, pgx.NamedArgs{
		"session_id":    c.SessionID,
		"provider_id":   c.ProviderID,
		"model":         c.Model,
		"tokens_input":  c.TokensInput,
		"tokens_output": c.TokensOutput,
		"cost_input":    c.CostInput,
		"cost_output":   c.CostOutput,
		"cost_total":    c.CostTotal,
		"cache_hit":     c.CacheHit,
	}).Scan(&c.ID, &c.CreatedAt)
}

// List returns cost logs matching the provided filter, paginated.
func (r *CostLogRepo) List(ctx context.Context, filter CostLogFilter) ([]CostLog, error) {
	query := `SELECT id, session_id, provider_id, model,
	                 tokens_input, tokens_output,
	                 cost_input, cost_output, cost_total, cache_hit, created_at
	          FROM cost_logs WHERE 1=1`
	args := make(map[string]interface{})

	if filter.ProviderID != nil {
		query += ` AND provider_id = @provider_id`
		args["provider_id"] = *filter.ProviderID
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

	var logs []CostLog
	for rows.Next() {
		var cl CostLog
		if err := rows.Scan(
			&cl.ID, &cl.SessionID, &cl.ProviderID, &cl.Model,
			&cl.TokensInput, &cl.TokensOutput,
			&cl.CostInput, &cl.CostOutput, &cl.CostTotal, &cl.CacheHit, &cl.CreatedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, cl)
	}
	return logs, rows.Err()
}

// GetSummary returns aggregated cost and token totals since the given time, grouped by provider.
func (r *CostLogRepo) GetSummary(ctx context.Context, since time.Time) (*CostSummary, error) {
	var summary CostSummary
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(cost_total), 0)::float8,
		       COALESCE(SUM(tokens_input + tokens_output), 0)::bigint
		FROM cost_logs
		WHERE created_at >= @since
	`, pgx.NamedArgs{"since": since}).Scan(&summary.TotalCost, &summary.TotalTokens)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT provider_id, SUM(cost_total)::float8
		FROM cost_logs
		WHERE created_at >= @since
		GROUP BY provider_id
	`, pgx.NamedArgs{"since": since})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary.ByProvider = make(map[string]float64)
	for rows.Next() {
		var providerID *uuid.UUID
		var cost float64
		if err := rows.Scan(&providerID, &cost); err != nil {
			return nil, err
		}
		key := ""
		if providerID != nil {
			key = providerID.String()
		}
		summary.ByProvider[key] = cost
	}
	return &summary, rows.Err()
}
