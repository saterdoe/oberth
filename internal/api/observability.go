package api

import (
	"context"
	"net/http"
	"time"
)

type readiness struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason"`
}

// BeginDrain closes admission before HTTP shutdown starts. Existing runs retain
// their durable leases and are recovered by the existing restart protocol.
func (s *Server) BeginDrain() { s.runsMu.Lock(); s.draining = true; s.runsMu.Unlock() }

func (s *Server) runtimeReadiness(ctx context.Context) readiness {
	s.runsMu.Lock()
	draining := s.draining
	s.runsMu.Unlock()
	if draining {
		return readiness{Reason: "draining"}
	}
	if s.pool == nil {
		return readiness{Reason: "database_unavailable"}
	}
	if s.executor == nil {
		return readiness{Reason: "runtime_unavailable"}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return readiness{Reason: "database_unavailable"}
	}
	defer tx.Rollback(ctx)
	// A no-row UPDATE checks write capability and expected schema without changing
	// user data; a successful SELECT 1 alone would accept a read-only database.
	if _, err = tx.Exec(ctx, `UPDATE task_runs SET version=version WHERE false`); err != nil {
		return readiness{Reason: "database_not_writable"}
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM providers WHERE is_active=true`).Scan(&active); err != nil {
		return readiness{Reason: "provider_configuration_unavailable"}
	}
	if active == 0 {
		return readiness{Reason: "no_active_provider"}
	}
	return readiness{Ready: true, Reason: "ready"}
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	result := s.runtimeReadiness(r.Context())
	status := http.StatusOK
	if !result.Ready {
		status = http.StatusServiceUnavailable
	}
	respondJSON(w, status, result)
}

type stuckRun struct {
	RunID         string    `json:"run_id"`
	TaskID        string    `json:"task_id"`
	LastProgress  time.Time `json:"last_progress"`
	LeaseExpired  bool      `json:"lease_expired"`
	ProgressStale bool      `json:"progress_stale"`
}

func (s *Server) handleRuntimeDiagnostics(w http.ResponseWriter, r *http.Request) {
	ready := s.runtimeReadiness(r.Context())
	signals := []stuckRun{}
	available := false
	if s.pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		rows, err := s.pool.Query(ctx, `SELECT id::text,task_id::text,COALESCE((SELECT MAX(created_at) FROM run_events WHERE run_id=task_runs.id),started_at),lease_expires_at<NOW() FROM task_runs WHERE state='running' ORDER BY started_at LIMIT 100`)
		if err == nil {
			available = true
			for rows.Next() {
				var item stuckRun
				if rows.Scan(&item.RunID, &item.TaskID, &item.LastProgress, &item.LeaseExpired) != nil {
					available = false
					break
				}
				item.ProgressStale = time.Since(item.LastProgress) > 5*time.Minute
				if item.LeaseExpired || item.ProgressStale {
					signals = append(signals, item)
				}
			}
			if rows.Err() != nil {
				available = false
			}
			rows.Close()
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{"schema_version": "1", "readiness": ready, "telemetry": s.telemetry.Snapshot(), "stuck_runs": signals, "stuck_query_available": available, "stuck_scan_limit": 100})
}
