package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type providerView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	IsActive     bool   `json:"is_active"`
	RateLimitRPM *int   `json:"rate_limit_rpm,omitempty"`
}

type providerModelsView struct {
	Models []string `json:"models"`
	Source string   `json:"source"`
}

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Configure model providers",
}

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured providers",
	RunE: func(_ *cobra.Command, _ []string) error {
		var providers []providerView
		if err := apiUnwrapGET("/providers", &providers); err != nil {
			return err
		}
		if len(providers) == 0 {
			fmt.Println("No providers configured.")
			return nil
		}
		for _, provider := range providers {
			status := "inactive"
			if provider.IsActive {
				status = "active"
			}
			fmt.Printf("[%s] %s (%s / %s)\n", status, provider.Name, provider.ProviderType, provider.DefaultModel)
		}
		return nil
	},
}

var providerAddFlags struct {
	Name      string
	Type      string
	BaseURL   string
	Model     string
	APIKeyEnv string
	RateLimit int
}

var providerAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add and activate a provider",
	RunE: func(_ *cobra.Command, _ []string) error {
		providerType := strings.ToLower(strings.TrimSpace(providerAddFlags.Type))
		if providerAddFlags.Name == "" || providerType == "" || providerAddFlags.Model == "" {
			return fmt.Errorf("--name, --type and --model are required")
		}
		if providerType != "openai" && providerType != "anthropic" && providerType != "ollama" &&
			providerType != "custom" && providerType != "google" && providerType != "vllm" && providerType != "tgi" {
			return fmt.Errorf("unsupported provider type %q", providerType)
		}
		if providerType != "openai" && providerType != "anthropic" && providerType != "ollama" &&
			strings.TrimSpace(providerAddFlags.BaseURL) == "" {
			return fmt.Errorf("--base-url is required for provider type %s", providerType)
		}
		apiKey := ""
		if providerAddFlags.APIKeyEnv != "" {
			apiKey = strings.TrimSpace(os.Getenv(providerAddFlags.APIKeyEnv))
			if apiKey == "" {
				return fmt.Errorf("environment variable %s is empty", providerAddFlags.APIKeyEnv)
			}
		}
		active := true
		body := map[string]any{
			"name": providerAddFlags.Name, "provider_type": providerType,
			"default_model": providerAddFlags.Model, "models": providerAddFlags.Model,
			"is_active": active,
		}
		if providerAddFlags.BaseURL != "" {
			body["base_url"] = providerAddFlags.BaseURL
		}
		if apiKey != "" {
			body["api_key"] = apiKey
		}
		if providerAddFlags.RateLimit > 0 {
			body["rate_limit_rpm"] = providerAddFlags.RateLimit
		}
		encoded, _ := json.Marshal(body)
		var created providerView
		if err := apiUnwrapPOST("/providers", string(encoded), &created); err != nil {
			return err
		}
		fmt.Printf("Provider %s added (%s / %s).\n", created.Name, created.ProviderType, created.DefaultModel)
		return nil
	},
}

var providerVerifyCmd = &cobra.Command{
	Use:   "verify <provider-id>",
	Short: "Verify provider reachability and list discovered models",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		var result providerModelsView
		if err := apiUnwrapPOST("/providers/"+args[0]+"/fetch-models", "{}", &result); err != nil {
			return fmt.Errorf("provider verification failed: %w", err)
		}
		if len(result.Models) == 0 {
			return fmt.Errorf("provider is reachable but returned no models")
		}
		fmt.Printf("Provider verified: %d model(s) discovered.\n", len(result.Models))
		for _, model := range result.Models {
			fmt.Printf("  %s\n", model)
		}
		return nil
	},
}

func init() {
	providerAddCmd.Flags().StringVar(&providerAddFlags.Name, "name", "", "provider display name")
	providerAddCmd.Flags().StringVar(&providerAddFlags.Type, "type", "", "openai, anthropic, ollama or compatible type")
	providerAddCmd.Flags().StringVar(&providerAddFlags.BaseURL, "base-url", "", "provider API base URL")
	providerAddCmd.Flags().StringVar(&providerAddFlags.Model, "model", "", "default model")
	providerAddCmd.Flags().StringVar(&providerAddFlags.APIKeyEnv, "api-key-env", "", "environment variable containing the API key")
	providerAddCmd.Flags().IntVar(&providerAddFlags.RateLimit, "rate-limit-rpm", 0, "optional requests-per-minute limit")
	providerCmd.AddCommand(providerListCmd, providerAddCmd, providerVerifyCmd)
	rootCmd.AddCommand(providerCmd)
}
