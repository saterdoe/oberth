package secrets

import (
	"strings"
	"testing"
)

func TestMarshalRedactedRemovesNestedSecrets(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	encoded, err := MarshalRedacted(map[string]any{
		"observation": map[string]any{
			"output": []string{"safe", "provider returned " + secret},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("secret remained in encoded evidence: %s", encoded)
	}
	if !strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("redaction marker missing: %s", encoded)
	}
}

func TestMarshalRedactedRemovesValuesBySensitiveKey(t *testing.T) {
	encoded, err := MarshalRedacted(map[string]any{
		"request": map[string]any{"password": "ordinary-looking-value", "nested": []any{map[string]any{"access_token": "opaque"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "ordinary-looking-value") || strings.Contains(text, "opaque") {
		t.Fatalf("structural secret remained: %s", text)
	}
}
