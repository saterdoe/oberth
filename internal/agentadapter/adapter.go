package agentadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = "1"

type Capabilities struct {
	SchemaVersion string   `json:"schema_version"`
	Agent         string   `json:"agent"`
	Transport     string   `json:"transport"`
	Features      []string `json:"features"`
	Installed     bool     `json:"installed"`
	Executable    string   `json:"executable,omitempty"`
	Version       string   `json:"version,omitempty"`
	Auth          string   `json:"auth"`
	Usable        bool     `json:"usable"`
	Evidence      []string `json:"evidence"`
	Message       string   `json:"message"`
}

type Request struct {
	SchemaVersion string          `json:"schema_version"`
	Worktree      string          `json:"worktree"`
	Intention     string          `json:"intention"`
	Context       json.RawMessage `json:"context"`
}

type Adapter interface {
	Name() string
	Capabilities(context.Context) (Capabilities, error)
	Execute(context.Context, Request) ([]byte, error)
}

type SupportedCLI struct {
	name       string
	executable string
}

func ClaudeCode() Adapter   { return SupportedCLI{name: "claude-code", executable: "claude"} }
func Codex() Adapter        { return SupportedCLI{name: "codex", executable: "codex"} }
func OpenCode() Adapter     { return SupportedCLI{name: "opencode", executable: "opencode"} }
func Antigravity() Adapter  { return SupportedCLI{name: "antigravity", executable: "antigravity"} }

func DefaultAgents() []Adapter {
	return []Adapter{
		Codex(),
		ClaudeCode(),
		OpenCode(),
		Antigravity(),
	}
}

func (a SupportedCLI) Name() string { return a.name }

func (a SupportedCLI) Capabilities(ctx context.Context) (Capabilities, error) {
	result := Capabilities{
		SchemaVersion: SchemaVersion,
		Agent:         a.name, Transport: "supported-cli", Auth: "unknown",
		Features: []string{"isolated_worktree", "durable_events", "result_bundle"},
		Evidence: []string{},
	}
	path, err := resolveExecutable(a.executable)
	if err != nil {
		result.Message = fmt.Sprintf("%s CLI no está instalado o no está en PATH", a.name)
		return result, nil
	}
	result.Installed = true
	result.Executable = path
	result.Evidence = append(result.Evidence, "executable_resolved")

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, probeErr := exec.CommandContext(probeCtx, path, "--version").CombinedOutput()
	if probeErr != nil {
		result.Message = "CLI detectado, pero el probe de versión falló; todavía no es ejecutable desde oberth"
		return result, nil
	}
	result.Version = strings.TrimSpace(string(out))
	result.Usable = result.Version != ""
	result.Auth = "not_verified"
	result.Evidence = append(result.Evidence, "version_probe_passed")
	if a.name == "claude-code" {
		authOut, authErr := exec.CommandContext(probeCtx, path, "auth", "status").CombinedOutput()
		var auth struct {
			LoggedIn   bool   `json:"loggedIn"`
			AuthMethod string `json:"authMethod"`
		}
		if json.Unmarshal(authOut, &auth) == nil {
			if auth.LoggedIn {
				result.Auth = auth.AuthMethod
				result.Evidence = append(result.Evidence, "auth_status_logged_in")
			} else {
				result.Auth = "required"
				result.Usable = false
				result.Evidence = append(result.Evidence, "auth_status_logged_out")
				result.Message = "Claude Code está instalado, pero requiere autenticación."
			}
		} else if authErr != nil {
			result.Auth = "probe_failed"
			result.Usable = false
		}
	}
	if result.Usable {
		result.Message = "CLI detectado. La autenticación se valida al iniciar una ejecución no interactiva."
	}
	return result, nil
}

// Execute uses only the agents' supported non-interactive interfaces. The
// caller supplies an isolated worktree and remains responsible for policy and
// durable event capture.
func (a SupportedCLI) Execute(ctx context.Context, request Request) ([]byte, error) {
	if request.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported adapter request schema %q", request.SchemaVersion)
	}
	if request.Worktree == "" || request.Intention == "" {
		return nil, fmt.Errorf("worktree and intention are required")
	}
	prompt := request.Intention
	if len(request.Context) > 0 {
		prompt += "\n\noberth context manifest:\n" + string(request.Context)
	}
	var command *exec.Cmd
	executable, err := resolveExecutable(a.executable)
	if err != nil {
		return nil, fmt.Errorf("%s CLI is not installed: %w", a.name, err)
	}
	switch a.name {
	case "claude-code":
		command = exec.CommandContext(ctx, executable, "-p", "--output-format", "json", prompt)
		command.Dir = request.Worktree
	case "codex":
		command = exec.CommandContext(ctx, executable, "exec", "--json", "-C", request.Worktree, prompt)
	case "opencode":
		command = exec.CommandContext(ctx, executable, "run", "--format", "json", prompt)
		command.Dir = request.Worktree
	case "antigravity":
		command = exec.CommandContext(ctx, executable, "run", "--json", prompt)
		command.Dir = request.Worktree
	default:
		return nil, fmt.Errorf("unsupported agent adapter %q", a.name)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s execution failed: %w", a.name, err)
	}
	return output, nil
}

func resolveExecutable(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(home, ".local", "bin", name),
		filepath.Join(home, ".local", "bin", name+".exe"),
		filepath.Join(home, "bin", name),
		filepath.Join(home, "bin", name+".exe"),
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s was not found in PATH or user-local bin directories", name)
}
