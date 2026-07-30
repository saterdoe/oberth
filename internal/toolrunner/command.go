package toolrunner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/permission"
)

var ErrApprovalRequired = errors.New("command requires approval")

type CommandLimits struct {
	Timeout        time.Duration
	MaxOutputBytes int
}

type Command struct {
	Program  string
	Args     []string
	Cwd      string
	Env      []string
	OnOutput func([]byte)
}

type CommandResult struct {
	Status    string
	ExitCode  int
	Output    string
	Truncated bool
	Duration  time.Duration
}

type CommandRunner struct {
	root   string
	policy *permission.Engine
	limits CommandLimits
}

func NewCommandRunner(root string, policy *permission.Engine, limits CommandLimits) *CommandRunner {
	return &CommandRunner{root: filepath.Clean(root), policy: policy, limits: limits}
}

func (r *CommandRunner) Run(ctx context.Context, command Command) (CommandResult, error) {
	started := time.Now()
	result := CommandResult{Status: "denied", ExitCode: -1}
	guard, err := permission.NewWorkspaceGuard(r.root)
	if err != nil {
		return result, err
	}
	if err := guard.Authorize(command.Cwd); err != nil {
		return result, err
	}
	target := strings.TrimSpace(strings.Join(append([]string{command.Program}, command.Args...), " "))
	decision, _ := r.policy.Evaluate(permission.Request{Operation: "command.exec", Target: target, RepoPath: r.root})
	if decision != permission.Allow {
		return result, ErrApprovalRequired
	}
	timeout := r.limits.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command.Program, command.Args...)
	cmd.Dir = command.Cwd
	if len(command.Env) > 0 {
		cmd.Env = append(os.Environ(), command.Env...)
	}
	configureProcessTree(cmd)
	output := &streamBuffer{max: r.limits.MaxOutputBytes, callback: command.OnOutput}
	cmd.Stdout, cmd.Stderr = output, output
	if err = cmd.Start(); err == nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case <-runCtx.Done():
			killProcessTree(cmd)
			err = <-done
		}
	}
	result.Duration = time.Since(started)
	result.Output = output.String()
	result.Truncated = output.truncated
	if runCtx.Err() == context.DeadlineExceeded {
		result.Status = "timeout"
		return result, runCtx.Err()
	}
	if err != nil {
		result.Status = "failed"
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, err
	}
	result.Status, result.ExitCode = "passed", 0
	return result, nil
}

type streamBuffer struct {
	buffer    bytes.Buffer
	max       int
	truncated bool
	callback  func([]byte)
}

func (w *streamBuffer) Write(data []byte) (int, error) {
	if w.callback != nil {
		w.callback(append([]byte(nil), data...))
	}
	written := len(data)
	remaining := w.max - w.buffer.Len()
	if w.max <= 0 {
		remaining = len(data)
	}
	if remaining <= 0 {
		w.truncated = true
		return written, nil
	}
	if len(data) > remaining {
		data = data[:remaining]
		w.truncated = true
	}
	_, err := w.buffer.Write(data)
	return written, err
}

func (w *streamBuffer) String() string { return w.buffer.String() }
