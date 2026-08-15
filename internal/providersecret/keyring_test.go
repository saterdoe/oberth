package providersecret

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveOrCreateFileKeyCreatesAndReusesPrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "provider.key")
	first, err := resolveOrCreateFileKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateKey(first); err != nil {
		t.Fatal(err)
	}
	second, err := resolveOrCreateFileKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("provider key fallback was not stable")
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("provider key fallback permissions are too broad: %o", info.Mode().Perm())
	}
}

func TestResolveOrCreateFileKeyRejectsInvalidExistingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.key")
	if err := os.WriteFile(path, []byte("not-a-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOrCreateFileKey(path); err == nil {
		t.Fatal("invalid provider key fallback must fail closed")
	}
}
