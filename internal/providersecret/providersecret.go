package providersecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const prefix = "enc:v1:"

// Seal encrypts a provider credential with a key derived from the daemon's
// local authentication token. AES-GCM provides confidentiality and detects
// corrupted or tampered ciphertext.
func Seal(authToken, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead, err := newAEAD(authToken)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate provider secret nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a credential. Values without the version prefix are returned
// unchanged so installations can read credentials written before encryption
// was introduced; the next explicit update stores them encrypted.
func Open(authToken, stored string) (string, error) {
	if stored == "" || !strings.HasPrefix(stored, prefix) {
		return stored, nil
	}
	aead, err := newAEAD(authToken)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", errors.New("provider secret has invalid encoding")
	}
	if len(raw) < aead.NonceSize() {
		return "", errors.New("provider secret is truncated")
	}
	plaintext, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], nil)
	if err != nil {
		return "", errors.New("provider secret could not be decrypted")
	}
	return string(plaintext), nil
}

func IsSealed(value string) bool {
	return strings.HasPrefix(value, prefix)
}

func newAEAD(authToken string) (cipher.AEAD, error) {
	if strings.TrimSpace(authToken) == "" {
		return nil, errors.New("local authentication token is required to protect provider secrets")
	}
	key := sha256.Sum256([]byte(authToken))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create provider secret cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
