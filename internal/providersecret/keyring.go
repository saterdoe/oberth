package providersecret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofrs/flock"
	keyring "github.com/zalando/go-keyring"
)

const keyringService = "oberth"
const keyringAccount = "provider-secret-key-v1"

func generateKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(raw), nil
}

func validateKey(value string) error {
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(raw) != 32 {
		return errors.New("provider key must be a base64-encoded 256-bit value")
	}
	return nil
}

func resolveOrCreateFileKey(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		value := strings.TrimSpace(string(data))
		if err := validateKey(value); err != nil {
			return "", fmt.Errorf("read provider key fallback: %w", err)
		}
		if err := protectFileKey(path); err != nil {
			return "", err
		}
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read provider key fallback: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create provider key directory: %w", err)
	}
	value, err := generateKey()
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".provider-secret-key-*")
	if err != nil {
		return "", fmt.Errorf("create provider key fallback: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return "", fmt.Errorf("protect provider key fallback: %w", err)
	}
	if _, err := file.WriteString(value + "\n"); err != nil {
		file.Close()
		return "", fmt.Errorf("write provider key fallback: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("sync provider key fallback: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close provider key fallback: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return resolveOrCreateFileKey(path)
		}
		return "", fmt.Errorf("publish provider key fallback: %w", err)
	}
	if err := protectFileKey(path); err != nil {
		return "", err
	}
	return value, nil
}

func protectFileKey(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect provider key fallback: %w", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("inspect provider key fallback: %w", err)
		}
		if info.Mode().Perm() != 0o600 {
			return fmt.Errorf("provider key fallback has unsafe permissions %04o", info.Mode().Perm())
		}
	}
	return nil
}

func fallbackKeyPath() (string, error) {
	if runtime.GOOS == "windows" {
		return "", errors.New("provider key file fallback is unavailable on Windows")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "oberth", keyringAccount), nil
}

func ResolveOrCreateKey() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate provider key lock: %w", err)
	}
	lockDir := filepath.Join(configDir, "oberth")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return "", fmt.Errorf("create provider key lock directory: %w", err)
	}
	keyLock := flock.New(filepath.Join(lockDir, ".provider-secret-key.lock"), flock.SetPermissions(0o600))
	if err := keyLock.Lock(); err != nil {
		return "", fmt.Errorf("lock provider key lifecycle: %w", err)
	}
	defer keyLock.Unlock()
	return resolveOrCreateKeyLocked()
}

func resolveOrCreateKeyLocked() (string, error) {
	value, err := keyring.Get(keyringService, keyringAccount)
	if err == nil && value != "" {
		if err := validateKey(value); err != nil {
			return "", fmt.Errorf("read provider key from OS credential store: %w", err)
		}
		return value, nil
	}
	path, pathErr := fallbackKeyPath()
	if pathErr == nil {
		if data, readErr := os.ReadFile(path); readErr == nil {
			fileValue := strings.TrimSpace(string(data))
			if validateErr := validateKey(fileValue); validateErr != nil {
				return "", fmt.Errorf("read provider key fallback: %w", validateErr)
			}
			if protectErr := protectFileKey(path); protectErr != nil {
				return "", protectErr
			}
			return fileValue, nil
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return "", fmt.Errorf("read provider key fallback: %w", readErr)
		}
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		if pathErr != nil {
			return "", fmt.Errorf("read provider key from OS credential store: %w", err)
		}
		return resolveOrCreateFileKey(path)
	}
	value, err = generateKey()
	if err != nil {
		return "", err
	}
	if err := keyring.Set(keyringService, keyringAccount, value); err != nil {
		if stored, getErr := keyring.Get(keyringService, keyringAccount); getErr == nil && stored != "" {
			if validateErr := validateKey(stored); validateErr != nil {
				return "", fmt.Errorf("verify provider key in OS credential store: %w", validateErr)
			}
			return stored, nil
		}
		if pathErr != nil {
			return "", fmt.Errorf("write provider key to OS credential store: %w", err)
		}
		return resolveOrCreateFileKey(path)
	}
	return value, nil
}
