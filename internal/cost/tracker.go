// Package cost provides cost tracking, budget management, and alerting for LLM API calls.
package cost

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/saterdoe/oberth/internal/db/repos"
)

// CallRecord contains details about an LLM API call to be recorded.
type CallRecord struct {
	SessionID    string
	ProviderID   string
	Model        string
	TokensInput  int
	TokensOutput int
	CostInput    float64
	CostOutput   float64
	CacheHit     bool
}

// BudgetAlert represents a budget threshold violation.
type BudgetAlert struct {
	BudgetID  string  `json:"budget_id"`
	Name      string  `json:"name"`
	UsagePct  float64 `json:"usage_pct"`
	Severity  string  `json:"severity"`
	HardLimit float64 `json:"hard_limit"`
}

// BudgetStatus represents the current status of a budget.
type BudgetStatus struct {
	BudgetID     string  `json:"budget_id"`
	Name         string  `json:"name"`
	CurrentSpend float64 `json:"current_spend"`
	SoftLimit    float64 `json:"soft_limit"`
	HardLimit    float64 `json:"hard_limit"`
	IsBlocked    bool    `json:"is_blocked"`
}

// Tracker handles cost logging, budget checks, and alerts.
type Tracker struct {
	costLogRepo   *repos.CostLogRepo
	budgetRepo    *repos.BudgetRepo
	auditRepo     *repos.AuditRepo
	reservationMu sync.Mutex
}

var ErrBudgetExceeded = errors.New("cost budget exceeded")

type Reservation struct {
	BudgetIDs []uuid.UUID
	Amount    float64
}

func (t *Tracker) Reserve(ctx context.Context, providerID string, amount float64) (*Reservation, error) {
	if t == nil || amount <= 0 {
		return &Reservation{}, nil
	}
	t.reservationMu.Lock()
	defer t.reservationMu.Unlock()
	budgets, err := t.budgetRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	var providerUUID uuid.UUID
	if providerID != "" {
		providerUUID, err = uuid.Parse(providerID)
		if err != nil {
			return nil, err
		}
	}
	reservation := &Reservation{Amount: amount}
	for _, budget := range budgets {
		if !budget.IsActive || (budget.ProviderID != nil && *budget.ProviderID != providerUUID) {
			continue
		}
		if budget.HardLimit > 0 && budget.CurrentSpend+amount > budget.HardLimit {
			return nil, ErrBudgetExceeded
		}
		reservation.BudgetIDs = append(reservation.BudgetIDs, budget.ID)
	}
	for _, id := range reservation.BudgetIDs {
		if err := t.budgetRepo.AddSpend(ctx, id, amount); err != nil {
			for _, applied := range reservation.BudgetIDs {
				if applied == id {
					break
				}
				_ = t.budgetRepo.AddSpend(ctx, applied, -amount)
			}
			return nil, err
		}
	}
	return reservation, nil
}

func (t *Tracker) Release(ctx context.Context, reservation *Reservation) {
	if t == nil || reservation == nil || reservation.Amount <= 0 {
		return
	}
	t.reservationMu.Lock()
	defer t.reservationMu.Unlock()
	for _, id := range reservation.BudgetIDs {
		_ = t.budgetRepo.AddSpend(ctx, id, -reservation.Amount)
	}
}

// NewTracker creates a new Tracker.
func NewTracker(costLogRepo *repos.CostLogRepo, budgetRepo *repos.BudgetRepo, auditRepo *repos.AuditRepo) *Tracker {
	return &Tracker{
		costLogRepo: costLogRepo,
		budgetRepo:  budgetRepo,
		auditRepo:   auditRepo,
	}
}

// RecordCall logs an LLM call and checks budgets.
// Returns an alert if budget thresholds are exceeded.
func (t *Tracker) RecordCall(ctx context.Context, call CallRecord) (*BudgetAlert, error) {
	sessionID, err := uuid.Parse(call.SessionID)
	if err != nil {
		return nil, err
	}

	var providerID *uuid.UUID
	if call.ProviderID != "" {
		pid, err := uuid.Parse(call.ProviderID)
		if err != nil {
			return nil, err
		}
		providerID = &pid
	}

	costLog := &repos.CostLog{
		SessionID:    sessionID,
		ProviderID:   providerID,
		Model:        call.Model,
		TokensInput:  call.TokensInput,
		TokensOutput: call.TokensOutput,
		CostInput:    call.CostInput,
		CostOutput:   call.CostOutput,
		CostTotal:    call.CostInput + call.CostOutput,
		CacheHit:     call.CacheHit,
	}

	if err := t.costLogRepo.Create(ctx, costLog); err != nil {
		return nil, err
	}

	budgets, err := t.budgetRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	var alert *BudgetAlert

	for _, budget := range budgets {
		if !budget.IsActive {
			continue
		}
		if budget.ProviderID != nil && (providerID == nil || *budget.ProviderID != *providerID) {
			continue
		}

		total := call.CostInput + call.CostOutput

		if err := t.budgetRepo.AddSpend(ctx, budget.ID, total); err != nil {
			slog.Warn("failed to add spend to budget", "budget_id", budget.ID, "error", err)
			continue
		}

		updated, err := t.budgetRepo.GetByID(ctx, budget.ID)
		if err != nil {
			slog.Warn("failed to fetch updated budget", "budget_id", budget.ID, "error", err)
			continue
		}

		if updated.HardLimit > 0 && updated.CurrentSpend >= updated.HardLimit {
			pct := updated.CurrentSpend / updated.HardLimit
			if pct > 1.0 {
				pct = 1.0
			}
			alert = &BudgetAlert{
				BudgetID:  updated.ID.String(),
				Name:      updated.Name,
				UsagePct:  pct,
				Severity:  "critical",
				HardLimit: updated.HardLimit,
			}
		} else if updated.SoftLimit > 0 && updated.CurrentSpend >= updated.SoftLimit {
			var pct float64
			if updated.HardLimit > 0 {
				pct = updated.CurrentSpend / updated.HardLimit
			}
			alert = &BudgetAlert{
				BudgetID:  updated.ID.String(),
				Name:      updated.Name,
				UsagePct:  pct,
				Severity:  "warning",
				HardLimit: updated.HardLimit,
			}
		}
	}

	details, _ := json.Marshal(map[string]any{
		"cost_log_id": costLog.ID.String(),
		"model":       call.Model,
		"cost_total":  costLog.CostTotal,
		"tokens":      call.TokensInput + call.TokensOutput,
	})

	auditEntry := &repos.AuditLogEntry{
		SessionID: &sessionID,
		Action:    "cost.call_recorded",
		Actor:     "system",
		Details:   details,
	}

	if err := t.auditRepo.Create(ctx, auditEntry); err != nil {
		slog.Warn("failed to log audit entry", "error", err)
	}

	return alert, nil
}

// CheckBudget checks if a provider has exceeded its budget.
// Returns the budget status without recording a call.
func (t *Tracker) CheckBudget(ctx context.Context, providerID string) (*BudgetStatus, error) {
	budgets, err := t.budgetRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	var providerUUID *uuid.UUID
	if providerID != "" {
		pid, err := uuid.Parse(providerID)
		if err != nil {
			return nil, err
		}
		providerUUID = &pid
	}

	for _, budget := range budgets {
		if !budget.IsActive {
			continue
		}
		if budget.ProviderID != nil && (providerUUID == nil || *budget.ProviderID != *providerUUID) {
			continue
		}

		return &BudgetStatus{
			BudgetID:     budget.ID.String(),
			Name:         budget.Name,
			CurrentSpend: budget.CurrentSpend,
			SoftLimit:    budget.SoftLimit,
			HardLimit:    budget.HardLimit,
			IsBlocked:    budget.HardLimit > 0 && budget.CurrentSpend >= budget.HardLimit,
		}, nil
	}

	return nil, nil
}
