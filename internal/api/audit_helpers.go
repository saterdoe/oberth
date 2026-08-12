package api

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/saterdoe/oberth/internal/db/repos"
	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

func (s *Server) logAudit(ctx context.Context, sessionID *uuid.UUID, action, actor string, details map[string]any) {
	if err := s.writeAudit(ctx, sessionID, action, actor, details); err != nil {
		slog.Warn("failed to write audit log", "action", action, "error", err)
	}
}

func (s *Server) writeAudit(ctx context.Context, sessionID *uuid.UUID, action, actor string, details map[string]any) error {
	if s.audit == nil {
		return fmt.Errorf("audit repository is unavailable")
	}
	data, err := secretspkg.MarshalRedacted(details)
	if err != nil {
		return fmt.Errorf("redact audit details: %w", err)
	}
	target := auditDetail(details, "target", "repo_path", "repository", "worktree")
	decision := auditDetail(details, "decision", "status", "outcome")
	correlationID := uuid.New()
	if raw := auditDetail(details, "correlation_id", "run_id", "task_id"); raw != "" {
		if parsed, parseErr := uuid.Parse(raw); parseErr == nil {
			correlationID = parsed
		}
	}
	entry := &repos.AuditLogEntry{
		SessionID:     sessionID,
		Action:        action,
		Actor:         actor,
		Target:        target,
		Decision:      decision,
		CorrelationID: correlationID,
		Details:       data,
	}
	return s.audit.Create(ctx, entry)
}

func auditDetail(details map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := fmt.Sprint(details[key]); value != "" && value != "<nil>" {
			return value
		}
	}
	return "unspecified"
}
