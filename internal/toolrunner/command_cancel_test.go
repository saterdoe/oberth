package toolrunner

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/saterdoe/oberth/internal/permission"
)

func TestCommandRunnerCancellationStopsPromptly(t *testing.T) {
	if os.Getenv("OBERTH_COMMAND_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	engine := permission.New()
	engine.AddRule(permission.Rule{
		Name: "test command", Priority: 100, Operation: "command.exec",
		TargetPattern: "*", Decision: permission.Allow,
	})
	root := t.TempDir()
	runner := NewCommandRunner(root, engine, CommandLimits{Timeout: time.Minute})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := runner.Run(ctx, Command{
		Program: os.Args[0],
		Args:    []string{"-test.run=TestCommandRunnerCancellationStopsPromptly"},
		Cwd:     root,
		Env:     []string{"OBERTH_COMMAND_HELPER=1"},
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", ctx.Err())
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("cancellation took too long: %s", elapsed)
	}
	if result.Status != "timeout" {
		t.Fatalf("expected timeout status, got %q", result.Status)
	}
}
