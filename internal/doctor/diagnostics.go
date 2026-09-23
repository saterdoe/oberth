package doctor

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/observability"
	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

type Probe func() error

type daemonStatus struct {
	Database struct {
		State string `json:"state"`
	} `json:"database"`
	Providers struct {
		Active int `json:"active"`
		Health []struct {
			Active bool   `json:"active"`
			State  string `json:"state"`
		} `json:"health"`
	} `json:"providers"`
}

var subsystemOrder = []string{"config", "database", "permissions", "repository", "git", "provider", "vault", "index", "ports", "daemon", "commands"}

func ComprehensiveChecks(probes map[string]Probe) []Check {
	checks := make([]Check, 0, len(subsystemOrder))
	for _, name := range subsystemOrder {
		probe := probes[name]
		if probe == nil {
			checks = append(checks, Check{Name: name, Status: StatusWarn, Message: "probe is not configured"})
			continue
		}
		if err := probe(); err != nil {
			checks = append(checks, Check{Name: name, Status: StatusFail, Message: err.Error()})
			continue
		}
		checks = append(checks, Check{Name: name, Status: StatusPass, Message: "healthy"})
	}
	return checks
}

type BundleInput struct {
	Logs     string              `json:"logs"`
	Config   string              `json:"config"`
	Versions map[string]string   `json:"versions"`
	Health   []Check             `json:"health"`
	Errors   []string            `json:"errors"`
	Runtime  *RuntimeDiagnostics `json:"runtime,omitempty"`
}

// Explicit fields keep source, prompts, output and credentials out of telemetry.
type RuntimeDiagnostics struct {
	SchemaVersion string `json:"schema_version"`
	Readiness     struct {
		Ready  bool   `json:"ready"`
		Reason string `json:"reason"`
	} `json:"readiness"`
	Telemetry observability.Snapshot `json:"telemetry"`
	StuckRuns []struct {
		RunID         string    `json:"run_id"`
		TaskID        string    `json:"task_id"`
		LastProgress  time.Time `json:"last_progress"`
		LeaseExpired  bool      `json:"lease_expired"`
		ProgressStale bool      `json:"progress_stale"`
	} `json:"stuck_runs"`
	StuckQueryAvailable bool `json:"stuck_query_available"`
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[^\s]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|token|password|secret)["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,}]+)`),
}

func redact(value string) string {
	for _, pattern := range sensitivePatterns {
		value = pattern.ReplaceAllString(value, `${1}[REDACTED]`)
	}
	return secretspkg.Redact(value)
}

func redactedJSON(value any) ([]byte, error) {
	data, err := secretspkg.MarshalRedacted(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err = json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	var visit func(any) any
	visit = func(value any) any {
		switch current := value.(type) {
		case string:
			return redact(current)
		case map[string]any:
			for key, child := range current {
				current[key] = visit(child)
			}
		case []any:
			for index, child := range current {
				current[index] = visit(child)
			}
		}
		return value
	}
	return json.MarshalIndent(visit(decoded), "", "  ")
}

func CreateBundle(path string, input BundleInput) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	entries := map[string]any{
		"logs.txt": input.Logs, "config.txt": "[REDACTED] Raw configuration omitted; inspect local configuration separately.", "versions.json": input.Versions,
		"health.json": input.Health, "errors.json": input.Errors,
		"runtime.json": input.Runtime,
	}
	for name, value := range entries {
		var content string
		if text, ok := value.(string); ok {
			content = redact(text)
		} else {
			data, marshalErr := redactedJSON(value)
			if marshalErr != nil {
				archive.Close()
				file.Close()
				return marshalErr
			}
			content = string(data)
		}
		writer, createErr := archive.Create(name)
		if createErr != nil {
			archive.Close()
			file.Close()
			return createErr
		}
		if _, writeErr := writer.Write([]byte(content)); writeErr != nil {
			archive.Close()
			file.Close()
			return writeErr
		}
	}
	if err := archive.Close(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// FetchRuntimeDiagnostics uses the same local authentication as status probes.
// It does not read a vault or source repository to construct the bundle.
func FetchRuntimeDiagnostics() (*RuntimeDiagnostics, error) {
	statusURL := strings.TrimSpace(os.Getenv("OBERTH_DOCTOR_STATUS_URL"))
	if statusURL == "" {
		statusURL = "http://127.0.0.1:9090/api/v1/status"
	}
	request, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(statusURL, "/status")+"/diagnostics/runtime", nil)
	if err != nil {
		return nil, fmt.Errorf("invalid diagnostics URL")
	}
	token := strings.TrimSpace(os.Getenv("OBERTH_AUTH_TOKEN"))
	if token == "" {
		if data, err := os.ReadFile(filepath.Join("data", "local-token")); err == nil {
			token = strings.TrimSpace(string(data))
		}
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("runtime diagnostics unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runtime diagnostics returned HTTP %d", response.StatusCode)
	}
	var envelope struct {
		Data RuntimeDiagnostics `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&envelope); err != nil || envelope.Data.SchemaVersion != "1" {
		return nil, fmt.Errorf("invalid runtime diagnostics response")
	}
	return &envelope.Data, nil
}

func DefaultProbes(configPath, vaultPath string) map[string]Probe {
	cwd, _ := os.Getwd()
	return map[string]Probe{
		"config": func() error { return checkError(RuntimeConfigCheck(configPath)) },
		"database": func() error {
			status, err := fetchDaemonStatus()
			if err != nil {
				return err
			}
			if status.Database.State != "healthy" {
				return fmt.Errorf("database state is %s", status.Database.State)
			}
			return nil
		},
		"permissions": func() error {
			file, err := os.CreateTemp(cwd, ".oberth-doctor-*")
			if err == nil {
				name := file.Name()
				file.Close()
				os.Remove(name)
			}
			return err
		},
		"repository": func() error { _, err := os.Stat(cwd); return err },
		"git":        func() error { return commandAvailable("git") },
		"provider": func() error {
			status, err := fetchDaemonStatus()
			if err != nil {
				return err
			}
			if status.Providers.Active == 0 {
				return fmt.Errorf("no active provider; run `oberth provider add`")
			}
			for _, provider := range status.Providers.Health {
				if provider.Active && provider.State != "unavailable" {
					return nil
				}
			}
			return fmt.Errorf("all active provider endpoints are unavailable")
		},
		"vault":    func() error { return checkError(VaultStructureCheck(vaultPath)) },
		"index":    func() error { return optionalFile(filepath.Join(vaultPath, ".index")) },
		"ports":    func() error { return nil },
		"daemon":   daemonHealth,
		"commands": func() error { return commandAvailable("go") },
	}
}

func daemonHealth() error {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	healthURL := strings.TrimSpace(os.Getenv("OBERTH_DOCTOR_DAEMON_URL"))
	if healthURL == "" {
		healthURL = "http://127.0.0.1:9090/api/v1/health"
	}
	response, err := client.Get(healthURL)
	if err != nil {
		return fmt.Errorf("daemon unavailable on 127.0.0.1:9090: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon health returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var health struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(body, &health) != nil || health.Status != "ok" {
		return fmt.Errorf("daemon returned an invalid health response")
	}
	return nil
}

func fetchDaemonStatus() (daemonStatus, error) {
	statusURL := strings.TrimSpace(os.Getenv("OBERTH_DOCTOR_STATUS_URL"))
	if statusURL == "" {
		statusURL = "http://127.0.0.1:9090/api/v1/status"
	}
	request, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		return daemonStatus{}, err
	}
	token := strings.TrimSpace(os.Getenv("OBERTH_AUTH_TOKEN"))
	if token == "" {
		if encoded, readErr := os.ReadFile(filepath.Join("data", "local-token")); readErr == nil {
			token = strings.TrimSpace(string(encoded))
		}
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return daemonStatus{}, fmt.Errorf("daemon status unavailable: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if response.StatusCode != http.StatusOK {
		return daemonStatus{}, fmt.Errorf("daemon status returned %s", response.Status)
	}
	var envelope struct {
		Data daemonStatus `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return daemonStatus{}, fmt.Errorf("invalid daemon status: %w", err)
	}
	return envelope.Data, nil
}

func checkError(check Check) error {
	if check.Status == StatusFail {
		return fmt.Errorf("%s", check.Message)
	}
	return nil
}

func optionalFile(path string) error {
	if _, err := os.Stat(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func commandAvailable(name string) error {
	pathValue := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathValue) {
		for _, suffix := range []string{"", ".exe", ".cmd"} {
			if info, err := os.Stat(filepath.Join(dir, name+suffix)); err == nil && !info.IsDir() {
				return nil
			}
		}
	}
	return fmt.Errorf("required command %s not found in %s", name, strings.Join(filepath.SplitList(pathValue), string(os.PathListSeparator)))
}
