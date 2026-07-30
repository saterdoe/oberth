package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestResultBundleV1MatchesPublishedRequiredFields(t *testing.T) {
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "docs", "schemas", "result-bundle-v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("published schema is not valid JSON: %v", err)
	}

	fixture := ResultBundleV1{
		SchemaVersion: resultBundleSchemaVersion,
		RunID:         uuid.New(), TaskID: uuid.New(), SessionID: uuid.New(),
		Warnings: []string{}, Runtime: map[string]any{},
	}
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, required := range schema.Required {
		if _, ok := fields[required]; !ok {
			t.Errorf("ResultBundleV1 is missing schema-required field %q", required)
		}
	}
}
