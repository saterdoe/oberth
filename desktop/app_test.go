package main

import "testing"

func TestNewAppStartsInPreparingState(t *testing.T) {
	app := NewApp()
	cfg := app.RuntimeConfig()
	if !cfg.Desktop || cfg.State != "preparing" {
		t.Fatalf("unexpected initial runtime config: %+v", cfg)
	}
	if cfg.APIURL != "" || cfg.APIToken != "" || cfg.Error != "" {
		t.Fatalf("preparing state exposed premature runtime data: %+v", cfg)
	}
}

func TestRuntimeConfigPublishesEndpointOnlyWhenReady(t *testing.T) {
	app := NewApp()
	app.config = RuntimeConfig{
		APIURL:   "http://127.0.0.1:9090",
		APIToken: "token",
		Desktop:  true,
		State:    "ready",
	}

	cfg := app.RuntimeConfig()
	if cfg.State != "ready" || cfg.APIURL == "" || cfg.APIToken == "" {
		t.Fatalf("ready runtime configuration was incomplete: %+v", cfg)
	}
}
