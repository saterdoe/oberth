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
