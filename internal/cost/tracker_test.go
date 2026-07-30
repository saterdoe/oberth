package cost

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db/repos"
)

func setupTestTracker(t *testing.T) (*Tracker, *repos.BudgetRepo, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	costLogRepo := repos.NewCostLogRepo(pool)
	budgetRepo := repos.NewBudgetRepo(pool)
	auditRepo := repos.NewAuditRepo(pool)

	tracker := NewTracker(costLogRepo, budgetRepo, auditRepo)
	return tracker, budgetRepo, ctx
}

func createTestBudget(t *testing.T, budgetRepo *repos.BudgetRepo, ctx context.Context, providerID *uuid.UUID, softLimit, hardLimit float64) *repos.Budget {
	t.Helper()
	if providerID != nil {
		pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
		require.NoError(t, err)
		defer pool.Close()
		_, err = pool.Exec(ctx, `INSERT INTO providers (id,name,provider_type,default_model) VALUES ($1,$2,'ollama','qa') ON CONFLICT (id) DO NOTHING`, *providerID, "qa-"+providerID.String())
		require.NoError(t, err)
	}
	b := &repos.Budget{
		Name:        "test-budget-" + uuid.NewString()[:8],
		Description: "test budget for tracker tests",
		ProviderID:  providerID,
		SoftLimit:   softLimit,
		HardLimit:   hardLimit,
		Period:      "monthly",
		PeriodStart: time.Now(),
		IsActive:    true,
	}
	err := budgetRepo.Create(ctx, b)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = budgetRepo.Delete(ctx, b.ID)
	})
	return b
}

func createTestSessionID(t *testing.T) string {
	t.Helper()
	id := uuid.New()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `INSERT INTO sessions (id,task_type,status) VALUES ($1,'implementation','active')`, id)
	require.NoError(t, err)
	return id.String()
}

func TestRecordCall_CreatesCostLog(t *testing.T) {
	tracker, _, ctx := setupTestTracker(t)
	sessionID := createTestSessionID(t)

	alert, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    0.002,
		CostOutput:   0.001,
		CacheHit:     false,
	})

	require.NoError(t, err)
	assert.Nil(t, alert, "no budgets configured, expected no alert")
}

func TestRecordCall_UpdatesBudgetSpend(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	sessionID := createTestSessionID(t)

	_ = createTestBudget(t, budgetRepo, ctx, nil, 100, 200)

	alert, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    10,
		CostOutput:   5,
		CacheHit:     false,
	})

	require.NoError(t, err)
	assert.Nil(t, alert, "spend 15 is under soft limit 100")
}

func TestRecordCall_TriggersSoftLimitAlert(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	sessionID := createTestSessionID(t)

	_ = createTestBudget(t, budgetRepo, ctx, nil, 10, 200)

	alert, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    10,
		CostOutput:   5,
		CacheHit:     false,
	})

	require.NoError(t, err)
	require.NotNil(t, alert)
	assert.Equal(t, "warning", alert.Severity)
	assert.InDelta(t, 15.0/200.0, alert.UsagePct, 0.01)
}

func TestRecordCall_TriggersHardLimitAndBlocked(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	sessionID := createTestSessionID(t)

	budget := createTestBudget(t, budgetRepo, ctx, nil, 5, 10)

	// Record enough spend to exceed hard limit
	alert, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    8,
		CostOutput:   5,
		CacheHit:     false,
	})

	require.NoError(t, err)
	require.NotNil(t, alert)
	assert.Equal(t, "critical", alert.Severity)
	assert.Equal(t, budget.ID.String(), alert.BudgetID)
}

func TestRecordCall_MultipleCallsAccumulateSpend(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	sessionID := createTestSessionID(t)

	_ = createTestBudget(t, budgetRepo, ctx, nil, 5, 10)

	// First call: spend 4, under limits
	alert1, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    2,
		CostOutput:   2,
		CacheHit:     false,
	})
	require.NoError(t, err)
	assert.Nil(t, alert1)

	// Second call: spend 4, total 8, exceeds soft limit (5) but under hard limit (10)
	alert2, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    2,
		CostOutput:   2,
		CacheHit:     false,
	})
	require.NoError(t, err)
	require.NotNil(t, alert2)
	assert.Equal(t, "warning", alert2.Severity)
}

func TestRecordCall_NoBudgetsNoAlert(t *testing.T) {
	tracker, _, ctx := setupTestTracker(t)
	sessionID := createTestSessionID(t)

	alert, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "claude-3",
		TokensInput:  200,
		TokensOutput: 100,
		CostInput:    0.01,
		CostOutput:   0.005,
		CacheHit:     true,
	})

	require.NoError(t, err)
	assert.Nil(t, alert)
}

func TestCheckBudget_ReturnsCorrectStatus(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	b := createTestBudget(t, budgetRepo, ctx, nil, 50, 100)

	status, err := tracker.CheckBudget(ctx, "")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, b.ID.String(), status.BudgetID)
	assert.Equal(t, 50.0, status.SoftLimit)
	assert.Equal(t, 100.0, status.HardLimit)
	assert.False(t, status.IsBlocked)
}

func TestCheckBudget_HardLimitExceededIsBlocked(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	_ = createTestBudget(t, budgetRepo, ctx, nil, 5, 10)

	sessionID := createTestSessionID(t)
	_, err := tracker.RecordCall(ctx, CallRecord{
		SessionID:    sessionID,
		ProviderID:   "",
		Model:        "gpt-4",
		TokensInput:  100,
		TokensOutput: 50,
		CostInput:    8,
		CostOutput:   5,
		CacheHit:     false,
	})
	require.NoError(t, err)

	status, err := tracker.CheckBudget(ctx, "")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.IsBlocked)
}

func TestCheckBudget_MatchingProvider(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	providerID := uuid.New()
	_ = createTestBudget(t, budgetRepo, ctx, &providerID, 50, 100)

	status, err := tracker.CheckBudget(ctx, providerID.String())
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, 50.0, status.SoftLimit)
	assert.False(t, status.IsBlocked)
}

func TestCheckBudget_NoMatchingBudget(t *testing.T) {
	tracker, budgetRepo, ctx := setupTestTracker(t)
	providerID := uuid.New()
	otherProviderID := uuid.New()
	_ = createTestBudget(t, budgetRepo, ctx, &providerID, 50, 100)

	status, err := tracker.CheckBudget(ctx, otherProviderID.String())
	require.NoError(t, err)
	assert.Nil(t, status)
}
