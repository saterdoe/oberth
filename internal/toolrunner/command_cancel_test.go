package toolrunner

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/saterdoe/oberth/internal/permission"
)

func TestCommandRunnerUsesMinimalExplicitEnvironment(t *testing.T) {
	if os.Getenv("OBERTH_ENV_HELPER") == "1" {
		_, _ = os.Stdout.WriteString(os.Getenv("OBERTH_PARENT_SECRET") + "|" + os.Getenv("OBERTH_EXPLICIT"))
		return
	}
	t.Setenv("OBERTH_PARENT_SECRET", "must-not-cross")
	engine := permission.New()
	engine.AddRule(permission.Rule{Name: "test", Priority: 100, Operation: "command.exec", TargetPattern: "*", Decision: permission.Allow})
	root := t.TempDir()
	runner := NewCommandRunner(root, engine, CommandLimits{Timeout: time.Minute, MaxOutputBytes: 1024})
	result, err := runner.Run(context.Background(), Command{Program: os.Args[0], Args: []string{"-test.run=TestCommandRunnerUsesMinimalExplicitEnvironment"}, Cwd: root, Env: []string{"OBERTH_ENV_HELPER=1", "OBERTH_EXPLICIT=allowed"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Output, "|allowed") || strings.Contains(result.Output, "must-not-cross") {
		t.Fatalf("unexpected environment output %q", result.Output)
	}
	if result.Limits.TimeoutMillis != time.Minute.Milliseconds() || result.Limits.MaxOutputBytes != 1024 {
		t.Fatalf("effective limits missing from evidence: %+v", result.Limits)
	}
}

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
