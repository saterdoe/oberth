package toolrunner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/permission"
)

var ErrApprovalRequired = errors.New("command requires approval")

type CommandLimits struct {
	Timeout        time.Duration
	MaxOutputBytes int
	// AllowedEnv names the parent-process variables that may cross the command
	// boundary. An empty list intentionally means no inherited variables except
	// the small platform bootstrap set returned by minimalEnvironment.
	AllowedEnv []string
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
	Limits    EffectiveLimits
}

type EffectiveLimits struct {
	TimeoutMillis  int64    `json:"timeout_ms"`
	MaxOutputBytes int      `json:"max_output_bytes"`
	AllowedEnv     []string `json:"allowed_env"`
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
	result.Limits = EffectiveLimits{TimeoutMillis: timeout.Milliseconds(), MaxOutputBytes: r.limits.MaxOutputBytes, AllowedEnv: append([]string(nil), r.limits.AllowedEnv...)}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command.Program, command.Args...)
	cmd.Dir = command.Cwd
	cmd.Env = buildEnvironment(r.limits.AllowedEnv, command.Env)
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

func buildEnvironment(allowedNames, explicit []string) []string {
	allowed := map[string]struct{}{}
	for _, name := range append(minimalEnvironmentNames(), allowedNames...) {
		allowed[strings.ToUpper(strings.TrimSpace(name))] = struct{}{}
	}
	values := map[string]string{}
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if _, ok := allowed[strings.ToUpper(name)]; found && ok {
			values[strings.ToUpper(name)] = name + "=" + value
		}
	}
	for _, entry := range explicit {
		name, _, found := strings.Cut(entry, "=")
		if found {
			values[strings.ToUpper(name)] = entry
		}
	}
	result := make([]string, 0, len(values))
	for _, entry := range values {
		result = append(result, entry)
	}
	slices.Sort(result)
	return result
}
