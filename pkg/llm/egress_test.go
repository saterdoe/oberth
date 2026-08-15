package llm

import (
	"net/http"
	"net/url"
	"testing"
)

func TestValidateProviderURLDeniesMetadataPrivateAndUnsafeSchemes(t *testing.T) {
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://10.1.2.3/v1",
		"http://[fe80::1]/v1",
		"file:///etc/passwd",
		"http://user:secret@example.com/v1",
	} {
		if err := ValidateProviderURL(raw, EgressPolicy{}); err == nil {
			t.Fatalf("expected %q to be denied", raw)
		}
	}
}

func TestEgressClientRejectsCrossOriginRedirectBeforeSendingCredentials(t *testing.T) {
	client := NewEgressClient(0, EgressPolicy{})
	original, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1/models", nil)
	original.Header.Set("Authorization", "Bearer secret")
	redirected, _ := http.NewRequest(http.MethodGet, "https://models.example.com/v1/models", nil)
	redirected.Header = original.Header.Clone()
	if err := client.CheckRedirect(redirected, []*http.Request{original}); err == nil {
		t.Fatal("cross-origin redirect carrying provider credentials must be rejected")
	}
	same, _ := url.Parse("https://api.example.com/next")
	if !sameProviderOrigin(original.URL, same) {
		t.Fatal("same provider origin should remain eligible for a validated redirect")
	}
}

func TestValidateProviderURLAllowsOnlyExplicitLoopbackException(t *testing.T) {
	if err := ValidateProviderURL("http://127.0.0.1:11434/v1", EgressPolicy{}); err == nil {
		t.Fatal("loopback must be denied by default")
	}
	if err := ValidateProviderURL("http://localhost:11434/v1", EgressPolicy{AllowLoopback: true}); err != nil {
		t.Fatalf("explicit loopback exception rejected: %v", err)
	}
}
