package gateway

import (
	"fmt"
	"strings"

	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/pkg/llm"
)

// BuildProvider creates an LLM client from a persisted provider config.
func BuildProvider(p repos.Provider) (llm.Provider, error) {
	providerType := strings.ToLower(strings.TrimSpace(p.ProviderType))
	baseURL := ""
	if p.BaseURL != nil {
		baseURL = strings.TrimSpace(*p.BaseURL)
	}

	switch providerType {
	case "openai":
		resolved := normalizeOpenAIBaseURL(baseURL)
		if resolved == "" {
			resolved = "https://api.openai.com/v1"
		}
		if err := llm.ValidateProviderURL(resolved, llm.EgressPolicy{}); err != nil {
			return nil, err
		}
		return llm.NewRestrictedOpenAI(resolved, providerAPIKey(p)), nil
	case "google":
		if baseURL == "" {
			return nil, fmt.Errorf("provider type %q requires base_url", p.ProviderType)
		}
		resolved := normalizeOpenAIBaseURL(baseURL)
		if err := llm.ValidateProviderURL(resolved, llm.EgressPolicy{}); err != nil {
			return nil, err
		}
		return llm.NewRestrictedOpenAI(resolved, providerAPIKey(p)), nil
	case "custom", "vllm", "tgi":
		if baseURL == "" {
			return nil, fmt.Errorf("provider type %q requires base_url", p.ProviderType)
		}
		resolved := normalizeOpenAIBaseURL(baseURL)
		// These provider types are the explicit local-network exception. Private
		// subnets remain denied; only loopback is permitted.
		policy := llm.EgressPolicy{AllowLoopback: true}
		if err := llm.ValidateProviderURL(resolved, policy); err != nil {
			return nil, err
		}
		return llm.NewOpenAI(resolved, providerAPIKey(p)), nil
	case "ollama":
		resolved := normalizeOpenAIBaseURL(baseURL)
		if resolved == "" {
			resolved = "http://localhost:11434/v1"
		}
		if err := llm.ValidateProviderURL(resolved, llm.EgressPolicy{AllowLoopback: true}); err != nil {
			return nil, err
		}
		return llm.NewOllama(resolved), nil
	case "anthropic":
		return llm.NewAnthropic(providerAPIKey(p)), nil
	default:
		return nil, fmt.Errorf("unsupported provider type %q", p.ProviderType)
	}
}

func normalizeOpenAIBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	if !strings.HasSuffix(baseURL, "/v1") {
		return strings.TrimRight(baseURL, "/") + "/v1"
	}
	return strings.TrimRight(baseURL, "/")
}

func providerAPIKey(p repos.Provider) string {
	if p.APIKeyEncrypted == nil {
		return ""
	}
	return *p.APIKeyEncrypted
}
