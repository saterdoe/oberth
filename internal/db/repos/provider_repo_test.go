//go:build integration

package repos

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db"
)

func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(context.Background(), `
		DO $$
		DECLARE table_record RECORD;
		BEGIN
			FOR table_record IN
				SELECT tablename FROM pg_tables
				WHERE schemaname = 'public' AND tablename <> 'schema_migrations'
			LOOP
				EXECUTE 'TRUNCATE TABLE public.' || quote_ident(table_record.tablename) || ' CASCADE';
			END LOOP;
		END $$`)
	require.NoError(t, err)
	return pool
}

func TestProviderRepo_CreateAndGetByID(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	p := &Provider{
		Name:         "test-provider-" + uuid.NewString()[:8],
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
		Priority:     10,
	}
	err := repo.Create(ctx, p)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, p.ID)
	require.False(t, p.CreatedAt.IsZero())

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, p.ProviderType, got.ProviderType)
	assert.Equal(t, p.DefaultModel, got.DefaultModel)
}

func TestProviderRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestProviderRepo_Update(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	p := &Provider{
		Name:         "update-test-" + uuid.NewString()[:8],
		ProviderType: "anthropic",
		DefaultModel: "claude-3-opus",
		IsActive:     true,
		Priority:     5,
	}
	require.NoError(t, repo.Create(ctx, p))

	p.Name = "updated-" + p.Name
	p.Priority = 1
	require.NoError(t, repo.Update(ctx, p))

	got, err := repo.GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, 1, got.Priority)
}

func TestProviderRepo_Update_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	err := repo.Update(ctx, &Provider{ID: uuid.New(), Name: "ghost"})
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestProviderRepo_Delete(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	p := &Provider{
		Name:         "delete-test-" + uuid.NewString()[:8],
		ProviderType: "ollama",
		DefaultModel: "llama3",
		IsActive:     true,
		Priority:     0,
	}
	require.NoError(t, repo.Create(ctx, p))
	require.NoError(t, repo.Delete(ctx, p.ID))

	_, err := repo.GetByID(ctx, p.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestProviderRepo_Delete_NotFound(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	err := repo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestProviderRepo_List(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		p := &Provider{
			Name:         "list-test-" + uuid.NewString()[:8],
			ProviderType: "openai",
			DefaultModel: "gpt-4o-mini",
			IsActive:     true,
			Priority:     i,
		}
		require.NoError(t, repo.Create(ctx, p))
	}

	providers, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(providers), 3)
}

func TestProviderRepo_List_Empty(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	providers, err := repo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, providers)
}

func TestProviderRepo_Create_AllowsDuplicateDisplayNames(t *testing.T) {
	pool := setupTestPool(t)
	repo := NewProviderRepo(pool)
	ctx := context.Background()

	name := "dup-test-" + uuid.NewString()[:8]
	p1 := &Provider{
		Name:         name,
		ProviderType: "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
		Priority:     1,
	}
	require.NoError(t, repo.Create(ctx, p1))

	p2 := &Provider{
		Name:         name,
		ProviderType: "anthropic",
		DefaultModel: "claude-3",
		IsActive:     true,
		Priority:     2,
	}
	err := repo.Create(ctx, p2)
	require.NoError(t, err)
}
