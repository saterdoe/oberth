//go:build integration

package db

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type MigrateTestSuite struct {
	suite.Suite
	db *sql.DB
}

func (s *MigrateTestSuite) SetupSuite() {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		s.T().Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	var err error
	s.db, err = sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.db.Ping())
}

func (s *MigrateTestSuite) TearDownSuite() {
	if s.db != nil {
		s.db.Close()
	}
}

func (s *MigrateTestSuite) TestMigrationsRunSuccessfully() {
	err := RunMigrations(s.db)
	require.NoError(s.T(), err)
}

func (s *MigrateTestSuite) TestMigrationsAreIdempotent() {
	err := RunMigrations(s.db)
	require.NoError(s.T(), err)
}

func TestMigrateSuite(t *testing.T) {
	suite.Run(t, new(MigrateTestSuite))
}
