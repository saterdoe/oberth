package verification

import (
	"context"
	"strings"

	"github.com/saterdoe/oberth/internal/toolrunner"
)

// SafeExecutor adapts the policy-governed Tool Runner to verification plans.
type SafeExecutor struct {
	runner *toolrunner.CommandRunner
}

func NewSafeExecutor(runner *toolrunner.CommandRunner) *SafeExecutor {
	return &SafeExecutor{runner: runner}
}

func (e *SafeExecutor) Execute(ctx context.Context, command, cwd string) Execution {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return Execution{ExitCode: -1, Output: "empty verification command"}
	}
	result, err := e.runner.Run(ctx, toolrunner.Command{Program: parts[0], Args: parts[1:], Cwd: cwd})
	output := result.Output
	if err != nil {
		if output != "" {
			output += "\n"
		}
		output += err.Error()
	}
	return Execution{ExitCode: result.ExitCode, Output: output, Duration: result.Duration}
}
