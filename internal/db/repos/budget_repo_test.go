//go:build integration

package repos

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db"
)

func TestBudgetRepo_CreateAndGetByID(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	b := &Budget{
		Name:        "test-budget-" + uuid.NewString()[:8],
		SoftLimit:   100.00,
		HardLimit:   200.00,
		Period:      "monthly",
		PeriodStart: time.Now(),
		IsActive:    true,
	}
	err := repo.Create(ctx, b)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, b.ID)

	got, err := repo.GetByID(ctx, b.ID)
	require.NoError(t, err)
	assert.Equal(t, b.ID, got.ID)
	assert.Equal(t, b.Name, got.Name)
	assert.InDelta(t, 100.00, got.SoftLimit, 0.01)
}

func TestBudgetReservationsEnforceLimitAndAreIdempotent(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()
	b := &Budget{Name: "reservation-" + uuid.NewString()[:8], HardLimit: 10, Period: "monthly", PeriodStart: time.Now(), IsActive: true}
	require.NoError(t, repo.Create(ctx, b))
	first, err := repo.Reserve(ctx, nil, 7, time.Minute)
	require.NoError(t, err)
	_, err = repo.Reserve(ctx, nil, 4, time.Minute)
	assert.ErrorIs(t, err, ErrBudgetReservationExceeded)
	require.NoError(t, repo.SetReservationState(ctx, first.ID, "released"))
	require.NoError(t, repo.SetReservationState(ctx, first.ID, "released"))
	_, err = repo.Reserve(ctx, nil, 4, time.Minute)
	require.NoError(t, err)
}

func TestBudgetReservationsReconcileExpiry(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()
	b := &Budget{Name: "expiry-" + uuid.NewString()[:8], HardLimit: 10, Period: "monthly", PeriodStart: time.Now(), IsActive: true}
	require.NoError(t, repo.Create(ctx, b))
	reservation, err := repo.Reserve(ctx, nil, 8, time.Nanosecond)
	require.NoError(t, err)
	time.Sleep(time.Millisecond)
	count, err := repo.ReconcileExpiredReservations(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(1))
	require.NoError(t, repo.SetReservationState(ctx, reservation.ID, "released"))
	_, err = repo.Reserve(ctx, nil, 8, time.Minute)
	require.NoError(t, err)
}

func TestBudgetRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestBudgetRepo_Update(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	b := &Budget{
		Name:        "update-budget-" + uuid.NewString()[:8],
		SoftLimit:   50.00,
		HardLimit:   100.00,
		Period:      "weekly",
		PeriodStart: time.Now(),
		IsActive:    true,
	}
	require.NoError(t, repo.Create(ctx, b))

	b.SoftLimit = 75.00
	b.IsActive = false
	require.NoError(t, repo.Update(ctx, b))

	got, err := repo.GetByID(ctx, b.ID)
	require.NoError(t, err)
	assert.InDelta(t, 75.00, got.SoftLimit, 0.01)
	assert.False(t, got.IsActive)
}

func TestBudgetRepo_Update_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	err := repo.Update(ctx, &Budget{
		ID:          uuid.New(),
		Name:        "ghost",
		SoftLimit:   10,
		HardLimit:   20,
		Period:      "monthly",
		PeriodStart: time.Now(),
	})
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestBudgetRepo_Delete(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	b := &Budget{
		Name:        "delete-budget-" + uuid.NewString()[:8],
		SoftLimit:   10.00,
		HardLimit:   20.00,
		Period:      "monthly",
		PeriodStart: time.Now(),
		IsActive:    true,
	}
	require.NoError(t, repo.Create(ctx, b))
	require.NoError(t, repo.Delete(ctx, b.ID))

	_, err := repo.GetByID(ctx, b.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestBudgetRepo_Delete_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	err := repo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestBudgetRepo_List(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		b := &Budget{
			Name:        "list-budget-" + uuid.NewString()[:8],
			SoftLimit:   100.00,
			HardLimit:   200.00,
			Period:      "monthly",
			PeriodStart: time.Now(),
			IsActive:    true,
		}
		require.NoError(t, repo.Create(ctx, b))
	}

	budgets, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(budgets), 3)
}

func TestBudgetRepo_AddSpend(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	b := &Budget{
		Name:         "addspend-budget-" + uuid.NewString()[:8],
		SoftLimit:    100.00,
		HardLimit:    200.00,
		Period:       "monthly",
		CurrentSpend: 0,
		PeriodStart:  time.Now(),
		IsActive:     true,
	}
	require.NoError(t, repo.Create(ctx, b))

	require.NoError(t, repo.AddSpend(ctx, b.ID, 25.50))
	require.NoError(t, repo.AddSpend(ctx, b.ID, 10.00))

	got, err := repo.GetByID(ctx, b.ID)
	require.NoError(t, err)
	assert.InDelta(t, 35.50, got.CurrentSpend, 0.01)
}

func TestBudgetRepo_AddSpend_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewBudgetRepo(pool)
	ctx := context.Background()

	err := repo.AddSpend(ctx, uuid.New(), 10.00)
	assert.ErrorIs(t, err, db.ErrNotFound)
}
