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
	Logs     string            `json:"logs"`
	Config   string            `json:"config"`
	Versions map[string]string `json:"versions"`
	Health   []Check           `json:"health"`
	Errors   []string          `json:"errors"`
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[^\s]+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|token|password)\s*[:=]\s*)[^\s]+`),
}

func redact(value string) string {
	for _, pattern := range sensitivePatterns {
		value = pattern.ReplaceAllString(value, `${1}[REDACTED]`)
	}
	return value
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
		"logs.txt": input.Logs, "config.txt": input.Config, "versions.json": input.Versions,
		"health.json": input.Health, "errors.json": input.Errors,
	}
	for name, value := range entries {
		var content string
		if text, ok := value.(string); ok {
			content = text
		} else {
			data, marshalErr := json.MarshalIndent(value, "", "  ")
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
		if _, writeErr := writer.Write([]byte(redact(content))); writeErr != nil {
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
