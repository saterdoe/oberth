package toolrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/saterdoe/oberth/internal/permission"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryRequiresCompleteToolContract(t *testing.T) {
	registry := NewRegistry(nil)
	err := registry.Register(Tool{Name: "incomplete"})
	assert.Error(t, err)

	err = registry.Register(Tool{
		Name: "echo", Schema: Schema{"text": "string"}, Permission: "tool.echo",
		Effect: EffectRead, Timeout: time.Second,
		Handler: func(_ context.Context, input Input) (Result, error) { return Result{Data: input}, nil },
	})
	require.NoError(t, err)
	result, err := registry.Execute(context.Background(), "echo", Input{"text": "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Data.(Input)["text"])
}

func TestRegistryRequestsApprovalForSensitiveEffects(t *testing.T) {
	policy := permission.New()
	registry := NewRegistry(policy)
	require.NoError(t, registry.Register(Tool{
		Name: "delete", Schema: Schema{"target": "string"}, Permission: "file.delete",
		Effect: EffectWrite, Timeout: time.Second,
		Handler: func(_ context.Context, _ Input) (Result, error) { return Result{}, nil },
	}))

	_, err := registry.Execute(context.Background(), "delete", Input{"target": "old.txt"})
	assert.ErrorIs(t, err, ErrApprovalRequired)
	policy.AddRule(permission.Rule{Name: "approved", Operation: "file.delete", TargetPattern: "old.txt", Decision: permission.Allow, Priority: 10})
	_, err = registry.Execute(context.Background(), "delete", Input{"target": "old.txt"})
	assert.NoError(t, err)
}

func TestRegistryAuditsAllowedDeniedAndFailedCalls(t *testing.T) {
	policy := permission.New()
	registry := NewRegistry(policy)
	var audit []AuditRecord
	registry.SetAuditSink(func(record AuditRecord) { audit = append(audit, record) })
	require.NoError(t, registry.Register(Tool{
		Name: "write", Schema: Schema{"target": "string"}, Permission: "file.write", Effect: EffectWrite, Timeout: time.Second,
		Handler: func(_ context.Context, _ Input) (Result, error) { return Result{}, nil },
	}))
	_, err := registry.Execute(context.Background(), "write", Input{"target": "a.txt"})
	assert.ErrorIs(t, err, ErrApprovalRequired)
	policy.AddRule(permission.Rule{Name: "allow", Operation: "file.write", TargetPattern: "a.txt", Decision: permission.Allow, Priority: 10})
	_, err = registry.Execute(context.Background(), "write", Input{"target": "a.txt"})
	require.NoError(t, err)
	require.Len(t, audit, 2)
	assert.Equal(t, "denied", audit[0].Status)
	assert.Equal(t, "passed", audit[1].Status)
	assert.Equal(t, EffectWrite, audit[1].Effect)
	assert.NotZero(t, audit[1].Duration)
}

func TestReadToolsStayInsideWorkspaceAndEnforceLimits(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha beta alpha"), 0600))
	guard, err := permission.NewWorkspaceGuard(root)
	require.NoError(t, err)
	reader := NewReader(root, guard, Limits{MaxFiles: 10, MaxBytes: 5, MaxMatches: 1})

	files, err := reader.List(".")
	require.NoError(t, err)
	assert.Equal(t, []string{"a.txt"}, files)
	content, truncated, err := reader.Read("a.txt")
	require.NoError(t, err)
	assert.Equal(t, "alpha", content)
	assert.True(t, truncated)
	matches, err := reader.Search("alpha")
	require.NoError(t, err)
	assert.Len(t, matches, 1)
	assert.ErrorIs(t, reader.Inspect(filepath.Join("..", "outside")), permission.ErrOutsideWorkspace)
}

func TestWriterCanAtomicallyReplaceAndRevertSession(t *testing.T) {
	root := t.TempDir()
	guard, err := permission.NewWorkspaceGuard(root)
	require.NoError(t, err)
	writer := NewWriter(root, guard)
	require.NoError(t, writer.Create("note.txt", []byte("one two three")))
	require.NoError(t, writer.ReplaceRange("session-1", "note.txt", 4, 7, []byte("TWO")))
	data, _ := os.ReadFile(filepath.Join(root, "note.txt"))
	assert.Equal(t, "one TWO three", string(data))
	require.NoError(t, writer.Revert("session-1"))
	data, _ = os.ReadFile(filepath.Join(root, "note.txt"))
	assert.Equal(t, "one two three", string(data))
}

func TestCommandRunnerControlsPolicyTimeoutOutputAndCwd(t *testing.T) {
	root := t.TempDir()
	engine := permission.New()
	engine.AddRule(permission.Rule{Name: "go", Operation: "command.exec", TargetPattern: "go version", Decision: permission.Allow, Priority: 10})
	runner := NewCommandRunner(root, engine, CommandLimits{Timeout: 10 * time.Second, MaxOutputBytes: 8})

	denied, err := runner.Run(context.Background(), Command{Program: "git", Args: []string{"status"}, Cwd: root})
	assert.ErrorIs(t, err, ErrApprovalRequired)
	assert.Equal(t, "denied", denied.Status)
	var streamed string
	result, err := runner.Run(context.Background(), Command{Program: "go", Args: []string{"version"}, Cwd: root, OnOutput: func(chunk []byte) { streamed += string(chunk) }})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, result.Truncated)
	assert.LessOrEqual(t, len(result.Output), 8)
	assert.NotZero(t, result.Duration)
	assert.Contains(t, streamed, "go version")
}
