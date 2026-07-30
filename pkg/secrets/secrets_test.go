package secrets

import (
	"strings"
	"testing"
)

func TestScanOpenAIKey(t *testing.T) {
	input := "my api key is sk-abc123def456ghi789jkl012 and more text"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	if r.Matches[0].Type != "OpenAI API Key" {
		t.Fatalf("expected OpenAI API Key, got %s", r.Matches[0].Type)
	}
}

func TestScanAnthropicKey(t *testing.T) {
	input := "key=sk-ant-abcdef1234567890abcdef1234567890"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
	if r.Matches[0].Type != "Anthropic API Key" {
		t.Fatalf("expected Anthropic API Key, got %s", r.Matches[0].Type)
	}
}

func TestScanAWSKey(t *testing.T) {
	input := "AKIA1234567890ABCDEF"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
	if r.Matches[0].Type != "AWS Access Key" {
		t.Fatalf("expected AWS Access Key, got %s", r.Matches[0].Type)
	}
}

func TestScanGitHubToken(t *testing.T) {
	input := "ghp_abc123def456ghi789jkl012mno345pqr"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
	if r.Matches[0].Type != "GitHub Token" {
		t.Fatalf("expected GitHub Token, got %s", r.Matches[0].Type)
	}
}

func TestScanPrivateKey(t *testing.T) {
	input := "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA..."
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
}

func TestScanPassword(t *testing.T) {
	input := `db_password = "supersecret123"`
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
	if r.Matches[0].Type != "Password Assignment" {
		t.Fatalf("expected Password Assignment, got %s", r.Matches[0].Type)
	}
}

func TestScanConnectionString(t *testing.T) {
	input := "postgres://user:pass123@localhost:5432/db"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
	if r.Matches[0].Type != "Connection String" {
		t.Fatalf("expected Connection String, got %s", r.Matches[0].Type)
	}
}

func TestScanJWT(t *testing.T) {
	input := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNqPZx2cT8eP5oF7qA"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a secret")
	}
	if r.Matches[0].Type != "JWT Token" {
		t.Fatalf("expected JWT Token, got %s", r.Matches[0].Type)
	}
}

func TestScanNoSecrets(t *testing.T) {
	input := "hello world this is a normal text without any secrets"
	r := Scan(input)
	if r.HasSecrets {
		t.Fatal("expected no secrets in normal text")
	}
}

func TestRedactOpenAIKey(t *testing.T) {
	input := "my key is sk-abc123def456ghi789jkl when needed"
	expected := "my key is [REDACTED] when needed"
	got := Redact(input)
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestRedactMultiple(t *testing.T) {
	input := "sk-abc123def456ghi789jkl and ghp_abc123def456ghi789jkl012mno345pqr"
	got := Redact(input)
	if got != "[REDACTED] and [REDACTED]" {
		t.Fatalf("expected both redacted, got %q", got)
	}
}

func TestRedactCleanText(t *testing.T) {
	input := "hello world"
	got := Redact(input)
	if got != input {
		t.Fatalf("expected unchanged text, got %q", got)
	}
}

func TestRedactWithType(t *testing.T) {
	input := "token=sk-abc123def456ghi789jkl012"
	result := RedactWithType(input)
	if result != "token=[REDACTED:OpenAI API Key]" {
		t.Fatalf("unexpected redaction: %q", result)
	}
}

func TestHasSecretsTrue(t *testing.T) {
	if !HasSecrets("key is sk-ant-abcdef1234567890abcdef") {
		t.Fatal("expected HasSecrets to return true")
	}
}

func TestHasSecretsFalse(t *testing.T) {
	if HasSecrets("just normal text") {
		t.Fatal("expected HasSecrets to return false")
	}
}

func TestSanitizeKey(t *testing.T) {
	key := "sk-abc123def456"
	got := SanitizeKey(key)
	expected := "sk" + strings.Repeat("*", len(key)-4) + "56"
	if got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestScanSlackToken(t *testing.T) {
	input := "xoxb-123456789012-abcdefghijklmnopqrst"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a Slack token")
	}
	if r.Matches[0].Type != "Slack Token" {
		t.Fatalf("expected Slack Token, got %s", r.Matches[0].Type)
	}
}

func TestScanGoogleServiceAccount(t *testing.T) {
	input := "my-service-account@my-project.iam.gserviceaccount.com"
	r := Scan(input)
	if !r.HasSecrets {
		t.Fatal("expected to find a service account email")
	}
	if r.Matches[0].Type != "Google Service Account" {
		t.Fatalf("expected Google Service Account, got %s", r.Matches[0].Type)
	}
}
