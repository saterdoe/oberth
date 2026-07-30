package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigrationFilesExist(t *testing.T) {
	upSQL, err := migrationsFS.ReadFile("migrations/000001_initial.up.sql")
	require.NoError(t, err, "should read up migration file")
	require.NotEmpty(t, upSQL, "up migration should not be empty")

	downSQL, err := migrationsFS.ReadFile("migrations/000001_initial.down.sql")
	require.NoError(t, err, "should read down migration file")
	require.NotEmpty(t, downSQL, "down migration should not be empty")
}
