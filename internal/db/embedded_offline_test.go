package db

import (
	"path/filepath"
	"testing"
)

func TestOfflineDatabaseRequiresPreparedDistribution(t *testing.T) {
	for _, binaries := range []string{"", "relative", filepath.Join(t.TempDir(), "missing")} {
		if database, err := StartEmbeddedOffline(t.TempDir(), binaries); err == nil || database != nil {
			t.Fatalf("missing distribution accepted: %q", binaries)
		}
	}
}
