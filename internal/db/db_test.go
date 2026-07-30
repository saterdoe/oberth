package db

import (
	"context"
	"testing"
)

func TestConnect_InvalidDSN(t *testing.T) {
	_, err := Connect(context.Background(), "invalid-dsn")
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

func TestConnect_ValidDSN(t *testing.T) {
	// This test only verifies that ParseConfig doesn't reject a valid DSN.
	// A real connection requires a running Postgres instance.
	_, err := Connect(context.Background(), "postgres://user:pass@localhost:5432/testdb?sslmode=disable")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
