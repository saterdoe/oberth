package main

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/saterdoe/oberth/internal/config"
)

func loadConfig(path string) (*config.Config, error) {
	cfg := config.Default()
	if path != "" {
		loaded, err := config.Load(path)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		cfg = loaded
	} else if err := config.ApplySemanticSettings(cfg); err != nil {
		slog.Warn("semantic search settings could not be loaded; using defaults", "error", err)
	}

	if token := strings.TrimSpace(os.Getenv("OBERTH_AUTH_TOKEN")); token != "" {
		cfg.Auth.Token = token
	}
	if host := strings.TrimSpace(os.Getenv("OBERTH_SERVER_HOST")); host != "" {
		cfg.Server.Host = host
	}
	if rawPort := strings.TrimSpace(os.Getenv("OBERTH_SERVER_PORT")); rawPort != "" {
		port, err := strconv.Atoi(rawPort)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("invalid OBERTH_SERVER_PORT %q", rawPort)
		}
		cfg.Server.Port = port
	}
	if driver := strings.TrimSpace(os.Getenv("OBERTH_DATABASE_DRIVER")); driver != "" {
		cfg.Database.Driver = driver
	}
	if dsn := strings.TrimSpace(os.Getenv("OBERTH_DATABASE_DSN")); dsn != "" {
		cfg.Database.DSN = dsn
		if strings.TrimSpace(os.Getenv("OBERTH_DATABASE_DRIVER")) == "" {
			cfg.Database.Driver = "postgres"
		}
	}
	if err := ensureLocalToken(cfg); err != nil {
		return nil, err
	}
	cfg.Auth.Mode = "token"
	return cfg, nil
}

func ensureLocalToken(cfg *config.Config) error {
	if strings.TrimSpace(cfg.Auth.Token) != "" {
		return nil
	}
	if persisted, err := os.ReadFile("data/local-token"); err == nil {
		cfg.Auth.Token = strings.TrimSpace(string(persisted))
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read local auth token: %w", err)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("failed to generate local auth token: %w", err)
	}
	cfg.Auth.Token = fmt.Sprintf("%x", raw)
	if err := os.MkdirAll("data", 0700); err != nil {
		return fmt.Errorf("failed to create local token directory: %w", err)
	}
	if err := os.WriteFile("data/local-token", []byte(cfg.Auth.Token+"\n"), 0600); err != nil {
		return fmt.Errorf("failed to persist local auth token: %w", err)
	}
	return nil
}

func configureLogger(level string) {
	logLevel := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))
}
