package main

import (
	"testing"

	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/providersecret"
)

func TestProviderWithOpenSecret(t *testing.T) {
	stored, err := providersecret.Seal("daemon-token", "provider-key")
	if err != nil {
		t.Fatal(err)
	}
	provider := repos.Provider{Name: "test", APIKeyEncrypted: &stored}

	opened, err := providerWithOpenSecret("daemon-token", provider)
	if err != nil {
		t.Fatal(err)
	}
	if opened.APIKeyEncrypted == nil || *opened.APIKeyEncrypted != "provider-key" {
		t.Fatalf("provider key was not decrypted: %+v", opened.APIKeyEncrypted)
	}
	if *provider.APIKeyEncrypted != stored {
		t.Fatal("persisted provider value was mutated")
	}
}

func TestProviderWithOpenSecretRejectsWrongToken(t *testing.T) {
	stored, err := providersecret.Seal("daemon-token", "provider-key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = providerWithOpenSecret("wrong-token", repos.Provider{Name: "test", APIKeyEncrypted: &stored})
	if err == nil {
		t.Fatal("expected decryption failure")
	}
}
