package providersecret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		_ = os.Chmod(path, 0o600)
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
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return resolveOrCreateFileKey(path)
	}
	if err != nil {
		return "", fmt.Errorf("create provider key fallback: %w", err)
	}
	if _, err := file.WriteString(value + "\n"); err != nil {
		file.Close()
		return "", fmt.Errorf("write provider key fallback: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close provider key fallback: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("protect provider key fallback: %w", err)
	}
	return value, nil
}

func ResolveOrCreateKey() (string, error) {
	value, err := keyring.Get(keyringService, keyringAccount)
	if err == nil && value != "" {
		return value, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		configDir, configErr := os.UserConfigDir()
		if configErr != nil {
			return "", fmt.Errorf("read provider key from OS credential store: %w", err)
		}
		return resolveOrCreateFileKey(filepath.Join(configDir, "oberth", keyringAccount))
	}
	value, err = generateKey()
	if err != nil {
		return "", err
	}
	if err := keyring.Set(keyringService, keyringAccount, value); err != nil {
		configDir, configErr := os.UserConfigDir()
		if configErr != nil {
			return "", fmt.Errorf("write provider key to OS credential store: %w", err)
		}
		return resolveOrCreateFileKey(filepath.Join(configDir, "oberth", keyringAccount))
	}
	return value, nil
}
