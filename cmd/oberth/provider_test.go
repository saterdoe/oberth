package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestProviderAddRejectsUnknownTypeBeforeAPICall(t *testing.T) {
	providerAddFlags.Name = "bad"
	providerAddFlags.Type = "mystery"
	providerAddFlags.Model = "model"
	err := providerAddCmd.RunE(providerAddCmd, nil)
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestProviderAddUsesEnvironmentSecretAndCreatesProvider(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/providers" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"data":{"id":"provider-1","name":"local","provider_type":"custom","default_model":"coder","is_active":true}}`))
	}))
	defer server.Close()
	parts := strings.Split(server.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	previousPort := apiPort
	apiPort = port
	defer func() { apiPort = previousPort }()

	t.Setenv("TEST_PROVIDER_KEY", "provider-secret-value")
	providerAddFlags.Name = "local"
	providerAddFlags.Type = "custom"
	providerAddFlags.Model = "coder"
	providerAddFlags.BaseURL = "http://127.0.0.1:1234"
	providerAddFlags.APIKeyEnv = "TEST_PROVIDER_KEY"
	defer func() {
		providerAddFlags = struct {
			Name      string
			Type      string
			BaseURL   string
			Model     string
			APIKeyEnv string
			RateLimit int
		}{}
	}()

	if err := providerAddCmd.RunE(providerAddCmd, nil); err != nil {
		t.Fatal(err)
	}
	if received["api_key"] != "provider-secret-value" {
		t.Fatalf("provider secret was not transmitted from the requested environment variable")
	}
}

func TestProviderVerifyDiscoversModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/providers/provider-1/fetch-models" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"models":["coder-small","coder-large"],"source":"api"}}`))
	}))
	defer server.Close()
	parts := strings.Split(server.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	previousPort := apiPort
	apiPort = port
	defer func() { apiPort = previousPort }()

	if err := providerVerifyCmd.RunE(providerVerifyCmd, []string{"provider-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestProviderVerifyRejectsEmptyModelCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"models":[],"source":"api"}}`))
	}))
	defer server.Close()
	parts := strings.Split(server.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	previousPort := apiPort
	apiPort = port
	defer func() { apiPort = previousPort }()

	if err := providerVerifyCmd.RunE(providerVerifyCmd, []string{"provider-1"}); err == nil {
		t.Fatal("expected empty model catalog to fail verification")
	}
}
