package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var apiPort = 9090

// APIError represents a structured error returned by the Oberth daemon.
type APIError struct {
	Status     int
	Code       string
	Message    string
	Details    json.RawMessage
	NextAction string
}

func (e *APIError) Error() string {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = http.StatusText(e.Status)
	}
	if e.Code != "" {
		message = e.Code + ": " + message
	}
	if e.NextAction != "" {
		message += "\nNext action: " + e.NextAction
	}
	return message
}

type authTransport struct{ base http.RoundTripper }

func (t authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if token := localAPIToken(); token != "" && req.Header.Get("Authorization") == "" {
		cloned := req.Clone(req.Context())
		cloned.Header = req.Header.Clone()
		cloned.Header.Set("Authorization", "Bearer "+token)
		req = cloned
	}
	return t.base.RoundTrip(req)
}

func init() {
	http.DefaultClient.Transport = authTransport{base: http.DefaultTransport}
}

func apiURL(path string) string {
	return fmt.Sprintf("http://localhost:%d/api/v1%s", apiPort, path)
}

func apiGET(path string) ([]byte, error) {
	return apiRequest("GET", path, "")
}

func apiPOST(path, body string) ([]byte, error) {
	return apiRequest("POST", path, body)
}

func apiRequest(method, path, body string) ([]byte, error) {
	url := apiURL(path)
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	token := localAPIToken()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		var envelope struct {
			Error struct {
				Code    string          `json:"code"`
				Message string          `json:"message"`
				Details json.RawMessage `json:"details"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &envelope)
		return nil, &APIError{
			Status: resp.StatusCode, Code: envelope.Error.Code,
			Message: envelope.Error.Message, Details: envelope.Error.Details,
			NextAction: apiErrorNextAction(resp.StatusCode, envelope.Error.Code),
		}
	}
	return data, nil
}

func apiErrorNextAction(status int, code string) string {
	switch code {
	case "UNAUTHORIZED":
		return "Restart oberth so the CLI and daemon use the same local token."
	case "INVALID_TRANSITION":
		return "Run `oberth status <run-id>` and choose an action valid for its current state."
	case "PROMOTION_FAILED":
		return "Resolve repository conflicts or dirty files, then retry approval."
	case "TASK_RUNNING":
		return "Cancel the active task before retrying this operation."
	case "NOT_FOUND":
		return "Check the ID with `oberth review` or omit it to use the latest run."
	case "NO_PROVIDER":
		return "Configure and activate a provider before starting the run."
	case "RUNTIME_UNAVAILABLE":
		return "Restart the daemon after configuring at least one provider."
	case "WORKTREE_FAILED":
		return "Ensure this is a clean Git repository and retry."
	}
	switch status {
	case http.StatusTooManyRequests:
		return "Wait for provider backoff or select another configured provider."
	case http.StatusUnauthorized, http.StatusForbidden:
		return "Check local authentication and the permission policy."
	case http.StatusServiceUnavailable:
		return "Run `oberth doctor` and verify daemon, database and provider health."
	default:
		return ""
	}
}

func asAPIError(err error) (*APIError, bool) {
	var target *APIError
	return target, errors.As(err, &target)
}

func localAPIToken() string {
	token := strings.TrimSpace(os.Getenv("OBERTH_AUTH_TOKEN"))
	if token == "" {
		if data, readErr := os.ReadFile(filepath.Join("data", "local-token")); readErr == nil {
			token = strings.TrimSpace(string(data))
		}
	}
	if token == "" {
		if configDir, err := os.UserConfigDir(); err == nil {
			if data, readErr := os.ReadFile(filepath.Join(configDir, "oberth", "local-token")); readErr == nil {
				token = strings.TrimSpace(string(data))
			}
		}
	}
	return token
}

// apiUnwrapGET performs a GET and returns the "data" field from the response.
func apiUnwrapGET(path string, dest any) error {
	raw, err := apiGET(path)
	if err != nil {
		return err
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	return json.Unmarshal(env.Data, dest)
}

// apiUnwrapPOST performs a POST and returns the "data" field from the response.
func apiUnwrapPOST(path, body string, dest any) error {
	raw, err := apiPOST(path, body)
	if err != nil {
		return err
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	return json.Unmarshal(env.Data, dest)
}
