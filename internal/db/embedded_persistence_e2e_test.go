//go:build e2e

package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEmbeddedStopReleasesDatabaseAndPreservesDataAcrossReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "postgres")
	first, err := StartEmbedded(root)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(context.Background(), first.DSN)
	if err != nil {
		_ = first.Stop()
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		CREATE TABLE native_restart_probe (id integer PRIMARY KEY, value text NOT NULL);
		INSERT INTO native_restart_probe(id,value) VALUES (1,'persisted')`); err != nil {
		pool.Close()
		_ = first.Stop()
		t.Fatal(err)
	}
	pool.Close()
	if err := first.Stop(); err != nil {
		t.Fatalf("graceful embedded shutdown failed: %v", err)
	}

	second, err := StartEmbedded(root)
	if err != nil {
		t.Fatalf("embedded database could not reopen after graceful shutdown: %v", err)
	}
	t.Cleanup(func() { _ = second.Stop() })
	reopened, err := Connect(context.Background(), second.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	var value string
	if err := reopened.QueryRow(context.Background(), `SELECT value FROM native_restart_probe WHERE id=1`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "persisted" {
		t.Fatalf("got %q after reopen, want persisted", value)
	}
}
