package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/internal/permission"
	workspacepkg "github.com/saterdoe/oberth/internal/workspace"
)

func TestHermeticAgentWorkspaceHappyPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "message.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	policy := permission.New()
	for _, operation := range []string{"file.write", "command.exec"} {
		policy.AddRule(permission.Rule{
			Name: operation, Priority: 100, Operation: operation,
			TargetPattern: "*", Decision: permission.Allow,
		})
	}
	ws, err := workspacepkg.NewRuntime("hermetic-test", root, policy)
	if err != nil {
		t.Fatal(err)
	}
	responses := []string{
		`{"schema_version":"1","tool":"read","arguments":{"path":"message.txt"}}`,
		`{"schema_version":"1","tool":"patch","arguments":{"path":"message.txt","old_text":"hello\n","new_text":"hello agent\n"}}`,
		`{"schema_version":"1","tool":"finish","arguments":{},"summary":"Updated the fixture."}`,
	}
	call := 0
	result, err := agentruntime.Run(context.Background(), "test", "update greeting", agentruntime.Config{
		MaxTurns: 4,
		Model: func(context.Context, []agentruntime.Message) (agentruntime.ModelResponse, error) {
			response := agentruntime.ModelResponse{Content: responses[call], Model: "fake"}
			call++
			return response, nil
		},
		Execute: func(ctx context.Context, action agentruntime.Action) agentruntime.Observation {
			return executeTypedTool(ctx, ws, "run", "task", "session", "implementation", "low", policy, nil, action)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "Updated the fixture." || len(result.Observations) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	content, err := os.ReadFile(filepath.Join(root, "message.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello agent\n" {
		t.Fatalf("unexpected workspace content %q", content)
	}
	var patchData map[string]any
	encoded, _ := json.Marshal(result.Observations[1].Data)
	if err := json.Unmarshal(encoded, &patchData); err != nil || patchData["change_set_id"] == "" {
		t.Fatalf("patch evidence missing: %v %+v", err, patchData)
	}
}
