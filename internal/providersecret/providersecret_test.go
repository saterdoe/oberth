package providersecret

import (
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	const token = "stable-local-token"
	const secret = "sk-sensitive-value"

	stored, err := Seal(token, secret)
	if err != nil {
		t.Fatal(err)
	}
	if !IsSealed(stored) || strings.Contains(stored, secret) {
		t.Fatalf("credential was not safely sealed: %q", stored)
	}
	opened, err := Open(token, stored)
	if err != nil {
		t.Fatal(err)
	}
	if opened != secret {
		t.Fatalf("got %q, want %q", opened, secret)
	}
}

func TestOpenRejectsWrongToken(t *testing.T) {
	stored, err := Seal("first-token", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open("different-token", stored); err == nil {
		t.Fatal("expected decryption to fail with a different token")
	}
}

func TestOpenSupportsLegacyPlaintext(t *testing.T) {
	opened, err := Open("", "legacy-secret")
	if err != nil {
		t.Fatal(err)
	}
	if opened != "legacy-secret" {
		t.Fatalf("got %q", opened)
	}
}
