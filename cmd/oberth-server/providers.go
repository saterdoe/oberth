package main

import (
	"fmt"

	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/providersecret"
)

func providerWithOpenSecret(authToken string, provider repos.Provider) (repos.Provider, error) {
	if provider.APIKeyEncrypted == nil {
		return provider, nil
	}
	plaintext, err := providersecret.Open(authToken, *provider.APIKeyEncrypted)
	if err != nil {
		return repos.Provider{}, fmt.Errorf("decrypt provider %q credential: %w", provider.Name, err)
	}
	// Keep the database model encrypted: callers receive a value copy containing
	// plaintext only for the short-lived provider construction step.
	provider.APIKeyEncrypted = &plaintext
	return provider, nil
}
