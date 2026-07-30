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
		return llm.NewOpenAI(normalizeOpenAIBaseURL(baseURL), providerAPIKey(p)), nil
	case "google", "custom", "vllm", "tgi":
		if baseURL == "" {
			return nil, fmt.Errorf("provider type %q requires base_url", p.ProviderType)
		}
		return llm.NewOpenAI(normalizeOpenAIBaseURL(baseURL), providerAPIKey(p)), nil
	case "ollama":
		return llm.NewOllama(normalizeOpenAIBaseURL(baseURL)), nil
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
