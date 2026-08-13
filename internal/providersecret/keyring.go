package providersecret

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	keyring "github.com/zalando/go-keyring"
)

const keyringService = "oberth"
const keyringAccount = "provider-secret-key-v1"

func ResolveOrCreateKey() (string, error) {
	value, err := keyring.Get(keyringService, keyringAccount)
	if err == nil && value != "" {
		return value, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("read provider key from OS credential store: %w", err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	value = base64.RawStdEncoding.EncodeToString(raw)
	if err := keyring.Set(keyringService, keyringAccount, value); err != nil {
		return "", fmt.Errorf("write provider key to OS credential store: %w", err)
	}
	return value, nil
}
