package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type SemanticSettings struct {
	Engine           string `json:"engine"`
	QdrantCollection string `json:"qdrant_collection,omitempty"`
}

func SemanticSettingsPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "oberth", "semantic-search.json")
	}
	return filepath.Join(".", "data", "semantic-search.json")
}

func ApplySemanticSettings(cfg *Config) error {
	data, err := os.ReadFile(SemanticSettingsPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read semantic settings: %w", err)
	}
	var settings SemanticSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("decode semantic settings: %w", err)
	}
	if settings.Engine != "" {
		cfg.VectorDB.Engine = settings.Engine
	}
	if settings.QdrantCollection != "" {
		cfg.VectorDB.Qdrant.Collection = settings.QdrantCollection
	}
	return nil
}

func SaveSemanticSettings(settings SemanticSettings) error {
	path := SemanticSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
