package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTextGolden(t *testing.T) {
	got := formatRunAcceptedText(runAccepted{
		RunID: "12345678-0000", TaskID: "abcdef12-0000", SessionID: "fedcba98-0000",
		Status: "accepted",
	}, runDetails{
		BaseRepository: "C:\\dev\\demo", BaseCommit: "112233445566", Branch: "oberth/run-1",
		WorktreePath: "C:\\data\\worktrees\\run-1",
	})
	assertGolden(t, "run-text.golden", got)
}

func TestReviewTextGolden(t *testing.T) {
	got := formatRunReviewText(runDetails{
		ID: "12345678-0000", State: "review",
		ResultBundle: json.RawMessage(`{"schema_version":"1","verification_status":"passed"}`),
	})
	assertGolden(t, "review-text.golden", got)
}

func TestStreamEventGolden(t *testing.T) {
	got := formatStreamEvent(streamEvent{
		RunID: "12345678-0000", Sequence: 7, Type: "tool.completed",
		Payload: json.RawMessage(`{"tool":"read","status":"ok"}`),
	}) + "\n"
	assertGolden(t, "stream-event.golden", got)
}

func TestRunStatusGolden(t *testing.T) {
	got := formatRunStatusText(runDetails{
		ID: "12345678-0000", TaskID: "abcdef12-0000", State: "blocked",
		ResultBundle: json.RawMessage(`{"cost":0.0123,"verification_status":"failed"}`),
	})
	assertGolden(t, "run-status.golden", got)
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	normalizedWant := strings.ReplaceAll(string(want), "\r\n", "\n")
	normalizedGot := strings.ReplaceAll(got, "\r\n", "\n")
	if normalizedGot != normalizedWant {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, got)
	}
}
