package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saterdoe/oberth/internal/nativepicker"
)

func TestCreateTaskRequiresExplicitRepository(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{
		"title":"Do work",
		"description":"Change a file"
	}`))

	server.handleCreateTask(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"REPOSITORY_REQUIRED"`) {
		t.Fatalf("missing actionable error code: %s", response.Body.String())
	}
}

func TestPickProjectDirectoryReturnsCanonicalGitRoot(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	server := &Server{pickDirectory: func(context.Context) (string, error) { return root, nil }}
	response := httptest.NewRecorder()

	server.handlePickProjectDirectory(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/pick-directory", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"canceled":false`) || !strings.Contains(response.Body.String(), `"path"`) {
		t.Fatalf("missing selected repository: %s", response.Body.String())
	}
}

func TestPickParentDirectoryAcceptsNonGitFolder(t *testing.T) {
	parent := t.TempDir()
	server := &Server{pickDirectory: func(context.Context) (string, error) { return parent, nil }}
	response := httptest.NewRecorder()

	server.handlePickParentDirectory(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/pick-parent-directory", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("got status %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"canceled":false`) || !strings.Contains(response.Body.String(), `"path"`) {
		t.Fatalf("missing selected parent: %s", response.Body.String())
	}
}

func TestPickProjectDirectoryTreatsCancelAsNonError(t *testing.T) {
	server := &Server{pickDirectory: func(context.Context) (string, error) {
		return "", nativepicker.ErrCanceled
	}}
	response := httptest.NewRecorder()

	server.handlePickProjectDirectory(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/pick-directory", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"canceled":true`) {
		t.Fatalf("unexpected cancel response %d: %s", response.Code, response.Body.String())
	}
}

func TestPickProjectDirectoryReportsUnavailablePicker(t *testing.T) {
	server := &Server{pickDirectory: func(context.Context) (string, error) {
		return "", errors.New("display failed")
	}}
	response := httptest.NewRecorder()

	server.handlePickProjectDirectory(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/pick-directory", nil))

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"PICKER_FAILED"`) {
		t.Fatalf("unexpected failure response %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateProjectRejectsNonGitPath(t *testing.T) {
	server := &Server{}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{
		"name":"not-a-repository",
		"path":"`+strings.ReplaceAll(t.TempDir(), `\`, `\\`)+`"
	}`))

	server.handleCreateProject(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"INVALID_REPOSITORY"`) {
		t.Fatalf("missing actionable error code: %s", response.Body.String())
	}
}

func TestInitializeNewProjectCreatesUsableGitRepository(t *testing.T) {
	parent := t.TempDir()
	target, err := initializeNewProject(context.Background(), parent, "from-scratch")
	if err != nil {
		t.Fatal(err)
	}
	expectedInfo, err := os.Stat(filepath.Join(parent, "from-scratch"))
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(expectedInfo, targetInfo) {
		t.Fatalf("unexpected target %q", target)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatalf("Git repository was not initialized: %v", err)
	}
	output, err := exec.Command("git", "-C", target, "rev-parse", "HEAD").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		t.Fatalf("initial commit missing: %v: %s", err, output)
	}
}
