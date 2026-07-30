package agentruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunExploresThenFinishes(t *testing.T) {
	responses := []string{
		`{"schema_version":"1","tool":"read","arguments":{"path":"main.go"}}`,
		`{"schema_version":"1","tool":"finish","arguments":{},"summary":"done"}`,
	}
	index := 0
	result, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns: 3,
		Model: func(context.Context, []Message) (ModelResponse, error) {
			response := ModelResponse{Content: responses[index], Model: "test", InputTokens: 2, OutputTokens: 1}
			index++
			return response, nil
		},
		Execute: func(_ context.Context, action Action) Observation {
			return Observation{Tool: action.Tool, Status: "ok", Data: "package main"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "done" || result.Turns != 2 || len(result.Observations) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseActionRejectsUnversionedOrUnknownTools(t *testing.T) {
	for _, input := range []string{
		`{"tool":"read","arguments":{}}`,
		`{"schema_version":"1","tool":"delete","arguments":{}}`,
	} {
		if _, err := ParseAction(input); err == nil {
			t.Fatalf("expected rejection for %s", input)
		}
	}
}

func TestParseActionAcceptsVersionedReasoningRecord(t *testing.T) {
	action, err := ParseAction(`{
		"schema_version":"1",
		"tool":"record_reasoning",
		"arguments":{
			"record":{
				"id":"h1",
				"kind":"hypothesis",
				"statement":"The parser drops empty values",
				"status":"open"
			}
		}
	}`)
	if err != nil || action.Tool != "record_reasoning" {
		t.Fatalf("expected versioned reasoning action, got %+v: %v", action, err)
	}
}

func TestParseActionAcceptsFirstOfAdjacentTypedActions(t *testing.T) {
	action, err := ParseAction(
		`{"schema_version":"1","tool":"patch","arguments":{"operation":"create","path":"docs/check.md","new_text":"ok\n"}}` +
			`{"schema_version":"1","tool":"command","arguments":{"program":"git","args":["diff","--check"]}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if action.Tool != "patch" {
		t.Fatalf("expected first complete typed action, got %+v", action)
	}
}

func TestRunRetriesMalformedTypedAction(t *testing.T) {
	responses := []string{
		`{"schema_version":"1",/"tool":"patch","arguments":{}}`,
		`{"schema_version":"1","tool":"finish","arguments":{"summary":"recovered"},"summary":"recovered"}`,
	}
	index := 0
	result, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns:         3,
		MaxFormatRetries: 1,
		Model: func(_ context.Context, messages []Message) (ModelResponse, error) {
			if index == 1 && !strings.Contains(messages[len(messages)-1].Content, "Parser error") {
				t.Fatal("expected corrective parser feedback")
			}
			response := ModelResponse{Content: responses[index]}
			index++
			return response, nil
		},
		Execute: func(context.Context, Action) Observation { return Observation{} },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "recovered" || result.JSONFallbacks != 1 {
		t.Fatalf("unexpected recovered result: %+v", result)
	}
}

func TestRunReturnsRecoverableModelFormatErrorAfterRetries(t *testing.T) {
	_, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns: 2, MaxFormatRetries: 1,
		Model: func(context.Context, []Message) (ModelResponse, error) {
			return ModelResponse{Content: "not json"}, nil
		},
		Execute: func(context.Context, Action) Observation { return Observation{} },
	})
	if !errors.Is(err, ErrModelFormatExhausted) {
		t.Fatalf("expected exhausted format error, got %v", err)
	}
}

func TestRunStopsLegitimatelyWhenEvidenceIsInsufficient(t *testing.T) {
	responses := []string{
		`{"schema_version":"1","tool":"record_reasoning","arguments":{"record":{"id":"u1","kind":"unknown","statement":"The deployed retry policy is unavailable","status":"unresolved","next_action":"inspect the deployed configuration"}}}`,
		`{"schema_version":"1","tool":"stop_insufficient_evidence","arguments":{"unknown_id":"u1","summary":"Cannot choose a safe retry strategy without production configuration."}}`,
	}
	index := 0
	result, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns: 3,
		Model: func(context.Context, []Message) (ModelResponse, error) {
			response := ModelResponse{Content: responses[index], Model: "test"}
			index++
			return response, nil
		},
		Execute: func(_ context.Context, action Action) Observation {
			return Observation{
				SchemaVersion: SchemaVersion, Tool: action.Tool, Status: "ok",
				Data: map[string]any{"record": map[string]any{
					"id": "u1", "kind": "unknown", "statement": "The deployed retry policy is unavailable",
					"status": "unresolved", "next_action": "inspect the deployed configuration",
				}},
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Termination != "insufficient_evidence" || result.UnknownID != "u1" || len(result.Observations) != 1 {
		t.Fatalf("unexpected terminal result: %+v", result)
	}
}

func TestRunRetriesStopThatReferencesUnrecordedUnknown(t *testing.T) {
	responses := []string{
		`{"schema_version":"1","tool":"stop_insufficient_evidence","arguments":{"unknown_id":"u-missing","summary":"Cannot continue."}}`,
		`{"schema_version":"1","tool":"finish","arguments":{},"summary":"continued instead"}`,
	}
	index := 0
	result, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns:           3,
		MaxProtocolRetries: 1,
		Model: func(_ context.Context, messages []Message) (ModelResponse, error) {
			if index == 1 && !strings.Contains(messages[len(messages)-1].Content, `unknown_id "u-missing" was not recorded`) {
				t.Fatal("expected corrective protocol feedback")
			}
			response := ModelResponse{Content: responses[index]}
			index++
			return response, nil
		},
		Execute: func(context.Context, Action) Observation { return Observation{} },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Termination != "" || result.Summary != "continued instead" || len(result.Actions) != 1 {
		t.Fatalf("unexpected recovered result: %+v", result)
	}
}

func TestRunRejectsFinishAfterFailedToolUntilModelRepairs(t *testing.T) {
	responses := []string{
		`{"schema_version":"1","tool":"patch","arguments":{"path":"docs/check.md","new_text":"ok"}}`,
		`{"schema_version":"1","tool":"finish","arguments":{"summary":"done"},"summary":"done"}`,
		`{"schema_version":"1","tool":"command","arguments":{"program":"git","args":["diff","--check"]}}`,
		`{"schema_version":"1","tool":"finish","arguments":{"summary":"verified"},"summary":"verified"}`,
	}
	index := 0
	result, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns: 5, MaxProtocolRetries: 1,
		Model: func(_ context.Context, messages []Message) (ModelResponse, error) {
			if index == 2 && !strings.Contains(messages[len(messages)-1].Content, "Do not claim success") {
				t.Fatal("expected failed-tool protocol correction")
			}
			response := ModelResponse{Content: responses[index]}
			index++
			return response, nil
		},
		Execute: func(_ context.Context, action Action) Observation {
			if action.Tool == "patch" {
				return Observation{Tool: action.Tool, Status: "failed", Error: "operation create is required"}
			}
			return Observation{Tool: action.Tool, Status: "ok"}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "verified" || len(result.Observations) != 2 {
		t.Fatalf("unexpected repaired result: %+v", result)
	}
}

func TestRunRejectsInsufficientEvidenceStopWithoutUnknownReference(t *testing.T) {
	_, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns: 1,
		Model: func(context.Context, []Message) (ModelResponse, error) {
			return ModelResponse{Content: `{"schema_version":"1","tool":"stop_insufficient_evidence","arguments":{"summary":"missing reference"}}`}, nil
		},
		Execute: func(context.Context, Action) Observation { return Observation{} },
	})
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected invalid terminal action, got %v", err)
	}
}

func TestRunPersistsObservationEnrichmentFromOnTurn(t *testing.T) {
	responses := []string{
		`{"schema_version":"1","tool":"read","arguments":{"path":"main.go"}}`,
		`{"schema_version":"1","tool":"finish","arguments":{},"summary":"done"}`,
	}
	index := 0
	result, err := Run(context.Background(), "system", "intent", Config{
		MaxTurns: 2,
		Model: func(context.Context, []Message) (ModelResponse, error) {
			response := ModelResponse{Content: responses[index]}
			index++
			return response, nil
		},
		Execute: func(context.Context, Action) Observation {
			return Observation{Tool: "read", Status: "ok", Data: "content"}
		},
		OnTurn: func(_ int, _ Action, observation *Observation) {
			if observation != nil {
				observation.Evidence = &Evidence{ID: "e1", Source: "file:main.go"}
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 1 || result.Observations[0].Evidence == nil ||
		result.Observations[0].Evidence.ID != "e1" {
		t.Fatalf("observation enrichment was lost: %+v", result.Observations)
	}
}
