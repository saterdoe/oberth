package providersecret

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	keyring "github.com/zalando/go-keyring"
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

func TestResolveOrCreateKeyConcurrentFirstUseSharesOneKey(t *testing.T) {
	keyring.MockInit()
	configDir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", configDir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", configDir)
	}
	_ = keyring.Delete(keyringService, keyringAccount)

	const callers = 16
	values := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := ResolveOrCreateKey()
			if err != nil {
				errs <- err
				return
			}
			values <- value
		}()
	}
	wg.Wait()
	close(values)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	stored, err := keyring.Get(keyringService, keyringAccount)
	if err != nil {
		t.Fatal(err)
	}
	for value := range values {
		if value != stored {
			t.Fatal("concurrent first-use callers did not converge on the persisted key")
		}
	}
}

func TestResolveOrCreateFileKeyConcurrentCallersShareOneKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "provider.key")
	const callers = 16
	values := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := resolveOrCreateFileKey(path)
			if err != nil {
				errs <- err
				return
			}
			values <- value
		}()
	}
	wg.Wait()
	close(values)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var expected string
	for value := range values {
		if expected == "" {
			expected = value
		}
		if value != expected {
			t.Fatal("concurrent callers resolved different provider keys")
		}
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
