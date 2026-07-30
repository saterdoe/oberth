package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/saterdoe/oberth/internal/nativepicker"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
)

type workspaceDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Color       string    `json:"color"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type projectDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	WorkspaceID *string   `json:"workspace_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type workspaceRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

type projectRequest struct {
	ID          string  `json:"id,omitempty"`
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	WorkspaceID *string `json:"workspace_id,omitempty"`
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id::text, name, description, color, created_at, updated_at
		FROM workspaces
		ORDER BY created_at ASC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list workspaces", nil)
		return
	}
	defer rows.Close()

	var out []workspaceDTO
	for rows.Next() {
		var item workspaceDTO
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Color, &item.CreatedAt, &item.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to scan workspace", nil)
			return
		}
		out = append(out, item)
	}
	if out == nil {
		out = []workspaceDTO{}
	}
	respondJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req workspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	if strings.TrimSpace(req.Color) == "" {
		req.Color = "var(--blue)"
	}
	id, err := parseOptionalUUID(req.ID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid workspace ID", nil)
		return
	}
	if id == nil {
		generated := uuid.New()
		id = &generated
	}
	var item workspaceDTO
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO workspaces (id, name, description, color)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name, description = EXCLUDED.description, color = EXCLUDED.color, updated_at = NOW()
		RETURNING id::text, name, description, color, created_at, updated_at`,
		*id, req.Name, req.Description, req.Color,
	).Scan(&item.ID, &item.Name, &item.Description, &item.Color, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create workspace", nil)
		return
	}
	respondJSON(w, http.StatusCreated, item)
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid workspace ID", nil)
		return
	}
	var req workspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required", nil)
		return
	}
	var item workspaceDTO
	err = s.pool.QueryRow(r.Context(), `
		UPDATE workspaces
		SET name = $2, description = $3, color = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING id::text, name, description, color, created_at, updated_at`,
		id, req.Name, req.Description, req.Color,
	).Scan(&item.ID, &item.Name, &item.Description, &item.Color, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "workspace not found", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update workspace", nil)
		return
	}
	respondJSON(w, http.StatusOK, item)
}

func (s *Server) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid workspace ID", nil)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM workspaces WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete workspace", nil)
		return
	}
	if tag.RowsAffected() == 0 {
		// Deletes are idempotent so an offline outbox can safely retry.
		respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `
		SELECT id::text, name, path, workspace_id::text, created_at, updated_at
		FROM projects
		ORDER BY created_at ASC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list projects", nil)
		return
	}
	defer rows.Close()
	var out []projectDTO
	for rows.Next() {
		var item projectDTO
		if err := rows.Scan(&item.ID, &item.Name, &item.Path, &item.WorkspaceID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to scan project", nil)
			return
		}
		out = append(out, item)
	}
	if out == nil {
		out = []projectDTO{}
	}
	respondJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req projectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Path = strings.TrimSpace(req.Path)
	if req.Name == "" || req.Path == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name and path are required", nil)
		return
	}
	repository, err := gitpkg.DetectRepo(req.Path)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REPOSITORY", "path must exist and belong to a Git repository", map[string]any{"path": req.Path})
		return
	}
	canonicalPath, err := filepath.EvalSymlinks(repository.Root)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REPOSITORY", "repository path could not be canonicalized", nil)
		return
	}
	req.Path, err = filepath.Abs(canonicalPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REPOSITORY", "repository path could not be resolved", nil)
		return
	}
	req.Path = filepath.Clean(req.Path)
	id, err := parseOptionalUUID(req.ID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid project ID", nil)
		return
	}
	if id == nil {
		generated := uuid.New()
		id = &generated
	}
	workspaceID, err := parseOptionalUUIDPtr(req.WorkspaceID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "invalid workspace ID", nil)
		return
	}
	var item projectDTO
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO projects (id, name, path, workspace_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (path) DO UPDATE
		SET name = EXCLUDED.name, workspace_id = EXCLUDED.workspace_id, updated_at = NOW()
		RETURNING id::text, name, path, workspace_id::text, created_at, updated_at`,
		*id, req.Name, req.Path, workspaceID,
	).Scan(&item.ID, &item.Name, &item.Path, &item.WorkspaceID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create project", nil)
		return
	}
	respondJSON(w, http.StatusCreated, item)
}

func (s *Server) handleCreateNewProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		ParentPath string `json:"parent_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.ParentPath = strings.TrimSpace(req.ParentPath)
	if req.Name == "" || req.ParentPath == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name and parent_path are required", nil)
		return
	}
	if req.Name != filepath.Base(req.Name) || req.Name == "." || req.Name == ".." {
		respondError(w, http.StatusBadRequest, "INVALID_PROJECT_NAME", "name must be a single folder name", nil)
		return
	}
	target, err := initializeNewProject(r.Context(), req.ParentPath, req.Name)
	if err != nil {
		status := http.StatusInternalServerError
		code := "PROJECT_CREATE_FAILED"
		if errors.Is(err, os.ErrExist) {
			status, code = http.StatusConflict, "PROJECT_PATH_EXISTS"
		} else if errors.Is(err, os.ErrInvalid) {
			status, code = http.StatusBadRequest, "INVALID_PARENT_PATH"
		}
		respondError(w, status, code, err.Error(), nil)
		return
	}
	var item projectDTO
	err = s.pool.QueryRow(r.Context(), `
		INSERT INTO projects (id, name, path)
		VALUES ($1, $2, $3)
		RETURNING id::text, name, path, workspace_id::text, created_at, updated_at`,
		uuid.New(), req.Name, filepath.Clean(target),
	).Scan(&item.ID, &item.Name, &item.Path, &item.WorkspaceID, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "project was initialized but could not be registered", map[string]any{"path": target})
		return
	}
	respondJSON(w, http.StatusCreated, item)
}

func initializeNewProject(ctx context.Context, parentPath, name string) (string, error) {
	parent, err := filepath.Abs(strings.TrimSpace(parentPath))
	if err != nil {
		return "", fmt.Errorf("%w: parent_path must be an absolute directory", os.ErrInvalid)
	}
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return "", fmt.Errorf("%w: parent_path must exist and be a directory", os.ErrInvalid)
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("%w: parent_path could not be canonicalized", os.ErrInvalid)
	}
	target := filepath.Join(parent, name)
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: the project folder already exists", os.ErrExist)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		return "", fmt.Errorf("create project folder: %w", err)
	}
	if err := gitpkg.InitializeRepository(ctx, target); err != nil {
		return "", err
	}
	return filepath.Clean(target), nil
}

func (s *Server) handlePickProjectDirectory(w http.ResponseWriter, r *http.Request) {
	if s.pickDirectory == nil {
		respondError(w, http.StatusServiceUnavailable, "PICKER_UNAVAILABLE", "native directory picker is unavailable", nil)
		return
	}
	selected, err := s.pickDirectory(r.Context())
	if err != nil {
		if errors.Is(err, nativepicker.ErrCanceled) {
			respondJSON(w, http.StatusOK, map[string]any{"canceled": true})
			return
		}
		if errors.Is(err, nativepicker.ErrUnavailable) {
			respondError(w, http.StatusServiceUnavailable, "PICKER_UNAVAILABLE", "native directory picker is unavailable", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "PICKER_FAILED", "native directory picker failed", nil)
		return
	}
	repository, err := gitpkg.DetectRepo(selected)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REPOSITORY", "selected folder must belong to a Git repository", nil)
		return
	}
	canonicalPath, err := filepath.EvalSymlinks(repository.Root)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REPOSITORY", "repository path could not be canonicalized", nil)
		return
	}
	canonicalPath, err = filepath.Abs(canonicalPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REPOSITORY", "repository path could not be resolved", nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"canceled": false,
		"path":     filepath.Clean(canonicalPath),
		"name":     filepath.Base(canonicalPath),
	})
}

func (s *Server) handlePickParentDirectory(w http.ResponseWriter, r *http.Request) {
	if s.pickDirectory == nil {
		respondError(w, http.StatusServiceUnavailable, "PICKER_UNAVAILABLE", "native directory picker is unavailable", nil)
		return
	}
	selected, err := s.pickDirectory(r.Context())
	if err != nil {
		if errors.Is(err, nativepicker.ErrCanceled) {
			respondJSON(w, http.StatusOK, map[string]any{"canceled": true})
			return
		}
		if errors.Is(err, nativepicker.ErrUnavailable) {
			respondError(w, http.StatusServiceUnavailable, "PICKER_UNAVAILABLE", "native directory picker is unavailable", nil)
			return
		}
		respondError(w, http.StatusInternalServerError, "PICKER_FAILED", "native directory picker failed", nil)
		return
	}
	info, err := os.Stat(selected)
	if err != nil || !info.IsDir() {
		respondError(w, http.StatusBadRequest, "INVALID_PARENT_PATH", "selected parent must be an existing directory", nil)
		return
	}
	canonicalPath, err := filepath.EvalSymlinks(selected)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_PARENT_PATH", "selected parent could not be canonicalized", nil)
		return
	}
	canonicalPath, err = filepath.Abs(canonicalPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_PARENT_PATH", "selected parent could not be resolved", nil)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"canceled": false,
		"path":     filepath.Clean(canonicalPath),
		"name":     filepath.Base(canonicalPath),
	})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "invalid project ID", nil)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete project", nil)
		return
	}
	if tag.RowsAffected() == 0 {
		// Deletes are idempotent so an offline outbox can safely retry.
		respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func parseOptionalUUID(value string) (*uuid.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseOptionalUUIDPtr(value *string) (*uuid.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	return parseOptionalUUID(*value)
}
