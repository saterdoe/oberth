package reasoning

import (
	"encoding/json"
	"testing"
)

func TestParseActionArgumentsValidatesUnknownAndProperty(t *testing.T) {
	for _, raw := range []string{
		`{"record":{"id":"u1","kind":"unknown","statement":"Production retry policy is unavailable","status":"unresolved","next_action":"read the deployed configuration"}}`,
		`{"record":{"id":"p1","kind":"property","statement":"Retries are idempotent","status":"unknown","required":true}}`,
		`{"evidence":{"id":"e1","source":"command:go test ./...","hash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`,
		`{"experiment":{"id":"x1","question":"Does the candidate pass the suite?","environment":"windows/amd64 go1.24","command":"go test ./...","expectation":"all packages pass","observation":"all packages passed","status":"passed","duration_ms":1250,"cost":0.001,"evidence_ids":["e1"],"claim_ids":["p1"],"baseline_fingerprint":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","candidate_fingerprint":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}`,
	} {
		if _, err := ParseActionArguments(json.RawMessage(raw)); err != nil {
			t.Fatalf("expected valid reasoning action %s: %v", raw, err)
		}
	}
}

func TestParseActionArgumentsRejectsUnfalsifiableShapes(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"record":{"id":"","kind":"fact","statement":"x","status":"supported"}}`,
		`{"record":{"id":"u1","kind":"unknown","statement":"x","status":"unresolved"}}`,
		`{"record":{"id":"p1","kind":"property","statement":"x","status":"open"}}`,
		`{"record":{"id":"h1","kind":"hypothesis","statement":"x","status":"open","confidence":1.2}}`,
		`{"record":{"id":"f1","kind":"fact","statement":"x","status":"supported"}}`,
		`{"evidence":{"id":"e1","source":"file:x","hash":"md5:abc"}}`,
		`{"experiment":{"id":"x1","question":"q","environment":"local","command":"go test","expectation":"pass","observation":"pass","status":"passed","evidence_ids":[]}}`,
		`{"experiment":{"id":"x1","question":"q","environment":"local","command":"go test","expectation":"pass","observation":"pass","status":"passed","evidence_ids":["e1"],"baseline_fingerprint":"sha256:a"}}`,
	} {
		if _, err := ParseActionArguments(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected invalid reasoning action %s", raw)
		}
	}
}

func TestCollectBuildsDeduplicatedCase(t *testing.T) {
	record := map[string]any{"record": map[string]any{
		"id": "h1", "kind": "hypothesis", "statement": "The parser drops empty values", "status": "supported",
		"evidence_ids": []string{"e1"}, "falsifier": "An empty value survives parsing",
	}}
	evidence := map[string]any{"evidence": map[string]any{
		"id": "e1", "source": "command:go test ./parser", "hash": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}}
	experiment := map[string]any{"experiment": map[string]any{
		"id": "x1", "question": "Does the parser pass?", "environment": "test",
		"command": "go test ./parser", "expectation": "pass", "observation": "passed",
		"status": "passed", "evidence_ids": []string{"e1"}, "claim_ids": []string{"h1"},
	}}
	result := Collect([]Observation{
		{Tool: "record_reasoning", Status: "ok", Data: record},
		{Tool: "record_reasoning", Status: "ok", Data: record},
		{Tool: "record_reasoning", Status: "ok", Data: evidence},
		{Tool: "record_reasoning", Status: "ok", Data: experiment},
		{Tool: "read", Status: "ok", Data: "ignored"},
	})
	if result == nil || result.SchemaVersion != SchemaVersion || len(result.Records) != 1 || len(result.Evidence) != 1 || len(result.Experiments) != 1 {
		t.Fatalf("unexpected reasoning case: %+v", result)
	}
}

func TestFindUnresolvedUnknownRequiresMatchingKindAndStatus(t *testing.T) {
	current := &CaseV1{Records: []Record{
		{ID: "u1", Kind: KindUnknown, Status: StatusUnresolved, Statement: "missing"},
		{ID: "u2", Kind: KindUnknown, Status: StatusSupported, Statement: "resolved"},
	}}
	if record, ok := FindUnresolvedUnknown(current, "u1"); !ok || record.Statement != "missing" {
		t.Fatalf("expected unresolved unknown, got %+v %v", record, ok)
	}
	if _, ok := FindUnresolvedUnknown(current, "u2"); ok {
		t.Fatal("resolved record must not authorize an insufficient-evidence stop")
	}
}

func TestAssessMeasuresCoverageAndRequiredGates(t *testing.T) {
	current := &CaseV1{
		Records: []Record{
			{ID: "h1", Kind: KindHypothesis, Status: StatusSupported, Statement: "parser drops values", EvidenceIDs: []string{"e1"}},
			{ID: "p1", Kind: KindProperty, Status: StatusUnknown, Statement: "retry is idempotent", Required: true},
		},
		Evidence: []EvidenceRef{{
			ID: "e1", Source: "file:parser.go",
			Hash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
	}
	assessment := Assess(current)
	if assessment.MaterialRecords != 2 || assessment.SupportedRecords != 1 || assessment.CoveragePercent != 50 {
		t.Fatalf("unexpected assessment: %+v", assessment)
	}
	if len(assessment.GateBlockers) != 2 {
		t.Fatalf("required unknown property must have evidence and pass: %+v", assessment.GateBlockers)
	}
}

func TestBindDiffEvidencePinsCommandEvidenceToCandidate(t *testing.T) {
	current := &CaseV1{Evidence: []EvidenceRef{{ID: "e1", Source: "command:go test ./...", Subject: "diff"}}}
	hash := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	BindDiffEvidence(current, hash)
	if current.Evidence[0].SubjectHash != hash {
		t.Fatalf("diff evidence was not bound: %+v", current.Evidence[0])
	}
}
