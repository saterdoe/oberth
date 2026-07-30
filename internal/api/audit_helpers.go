package api

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/saterdoe/oberth/internal/db/repos"
	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

func (s *Server) logAudit(ctx context.Context, sessionID *uuid.UUID, action, actor string, details map[string]any) {
	if s.audit == nil {
		return
	}
	data, err := secretspkg.MarshalRedacted(details)
	if err != nil {
		data = json.RawMessage(`{}`)
	}
	entry := &repos.AuditLogEntry{
		SessionID: sessionID,
		Action:    action,
		Actor:     actor,
		Details:   data,
	}
	if err := s.audit.Create(ctx, entry); err != nil {
		slog.Warn("failed to write audit log", "action", action, "error", err)
	}
}
