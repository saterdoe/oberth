package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/saterdoe/oberth/internal/reasoning"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

func (s *Server) handleExportRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid run ID", nil)
		return
	}
	var bundle json.RawMessage
	var state string
	if err := s.pool.QueryRow(r.Context(), `SELECT COALESCE(result_bundle,'{}'),state FROM task_runs WHERE id=$1`, id).Scan(&bundle, &state); err != nil {
		respondError(w, 404, "NOT_FOUND", "run not found", nil)
		return
	}
	if r.URL.Query().Get("format") == "markdown" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="oberth-run-`+id.String()+`.md"`)
		_, _ = w.Write([]byte(renderResultBundleMarkdown(id.String(), state, bundle)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="oberth-run-`+id.String()+`.json"`)
	w.Write(bundle)
}

func renderResultBundleMarkdown(runID, state string, bundle json.RawMessage) string {
	var value struct {
		SchemaVersion      string            `json:"schema_version"`
		BaseCommit         string            `json:"base_commit"`
		Branch             string            `json:"branch"`
		Diff               []gitpkg.DiffFile `json:"diff"`
		DiffHash           string            `json:"diff_hash"`
		ContextHash        string            `json:"context_hash"`
		Cost               float64           `json:"cost"`
		TokensInput        int               `json:"tokens_input"`
		TokensOutput       int               `json:"tokens_output"`
		Warnings           []string          `json:"warnings"`
		VerificationStatus any               `json:"verification_status"`
		Environment        EnvironmentV1     `json:"environment"`
		Reasoning          *reasoning.CaseV1 `json:"reasoning"`
		Outcome            string            `json:"outcome"`
	}
	_ = json.Unmarshal(bundle, &value)
	var output strings.Builder
	fmt.Fprintf(&output, "# oberth run %s\n\n", runID)
	fmt.Fprintf(&output, "- State: **%s**\n- Verification: **%v**\n- Cost: **$%.4f**\n", state, value.VerificationStatus, value.Cost)
	fmt.Fprintf(&output, "- Tokens: %d input / %d output\n- Base commit: `%s`\n- Branch: `%s`\n", value.TokensInput, value.TokensOutput, value.BaseCommit, value.Branch)
	fmt.Fprintf(&output, "- Environment: `%s/%s` (`%s`)\n- Diff hash: `%s`\n- Context hash: `%s`\n", value.Environment.OS, value.Environment.Arch, value.Environment.GoVersion, value.DiffHash, value.ContextHash)
	if value.Outcome != "" {
		fmt.Fprintf(&output, "- Human outcome: **%s**\n", value.Outcome)
	}
	if len(value.Warnings) > 0 {
		output.WriteString("\n## Warnings\n\n")
		for _, warning := range value.Warnings {
			fmt.Fprintf(&output, "- %s\n", warning)
		}
	}
	if value.Reasoning != nil {
		output.WriteString("\n## Reasoning evidence\n\n")
		fmt.Fprintf(&output, "- Coverage: **%.0f%%** (%d/%d material records supported)\n",
			value.Reasoning.Assessment.CoveragePercent,
			value.Reasoning.Assessment.SupportedRecords,
			value.Reasoning.Assessment.MaterialRecords)
		if len(value.Reasoning.Assessment.GateBlockers) > 0 {
			output.WriteString("- Promotion blockers:\n")
			for _, blocker := range value.Reasoning.Assessment.GateBlockers {
				fmt.Fprintf(&output, "  - %s\n", blocker)
			}
		}
		if len(value.Reasoning.Records) == 0 {
			output.WriteString("No reasoning records were captured.\n")
		}
		for _, record := range value.Reasoning.Records {
			fmt.Fprintf(&output, "- **%s · %s** — %s", record.Kind, record.Status, record.Statement)
			if len(record.EvidenceIDs) > 0 {
				fmt.Fprintf(&output, " (evidence: `%s`)", strings.Join(record.EvidenceIDs, "`, `"))
			}
			if record.Required {
				output.WriteString(" · required")
			}
			output.WriteString("\n")
			if record.Falsifier != "" {
				fmt.Fprintf(&output, "  - Falsifier: %s\n", record.Falsifier)
			}
			if record.NextAction != "" {
				fmt.Fprintf(&output, "  - Next action: %s\n", record.NextAction)
			}
		}
		if len(value.Reasoning.Evidence) > 0 {
			output.WriteString("\n### Evidence references\n\n")
			for _, evidence := range value.Reasoning.Evidence {
				fmt.Fprintf(&output, "- `%s`: %s", evidence.ID, evidence.Source)
				if evidence.Hash != "" {
					fmt.Fprintf(&output, " (`%s`)", evidence.Hash)
				}
				if evidence.Detail != "" {
					fmt.Fprintf(&output, " — %s", evidence.Detail)
				}
				if evidence.Stale {
					output.WriteString(" — **STALE**")
				}
				output.WriteString("\n")
			}
		}
		if len(value.Reasoning.Experiments) > 0 {
			output.WriteString("\n### Reproducible experiments\n\n")
			for _, experiment := range value.Reasoning.Experiments {
				fmt.Fprintf(&output, "- **%s · %s** — %s\n", experiment.ID, experiment.Status, experiment.Question)
				fmt.Fprintf(&output, "  - Environment: `%s`\n  - Command: `%s`\n", experiment.Environment, experiment.Command)
				fmt.Fprintf(&output, "  - Expected: %s\n  - Observed: %s\n", experiment.Expectation, experiment.Observation)
				fmt.Fprintf(&output, "  - Evidence: `%s` · claims: `%s` · %d ms · $%.4f\n",
					strings.Join(experiment.EvidenceIDs, "`, `"), strings.Join(experiment.ClaimIDs, "`, `"),
					experiment.DurationMS, experiment.Cost)
				if experiment.Baseline != "" {
					fmt.Fprintf(&output, "  - Comparison: `%s` → `%s`\n", experiment.Baseline, experiment.Candidate)
				}
			}
		}
	}
	output.WriteString("\n## Changes\n")
	if len(value.Diff) == 0 {
		output.WriteString("\nNo file changes were recorded.\n")
	}
	for _, file := range value.Diff {
		fmt.Fprintf(&output, "\n### `%s` (%s)\n\n````diff\n%s\n````\n", file.Path, file.Status, strings.TrimSpace(file.Content))
	}
	output.WriteString("\n## Contract\n\n")
	fmt.Fprintf(&output, "Result bundle schema `%s`. Evidence hashes allow consumers to detect drift.\n", value.SchemaVersion)
	return output.String()
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid run ID", nil)
		return
	}
	var result struct {
		ID, TaskID, SessionID, State, SchemaVersion, BaseRepository, BaseCommit, WorktreePath, Branch string
		ResultBundle                                                                                  json.RawMessage
	}
	err = s.pool.QueryRow(r.Context(), `SELECT id::text,task_id::text,session_id::text,state,schema_version,base_repository,base_commit,worktree_path,branch,COALESCE(result_bundle,'{}') FROM task_runs WHERE id=$1`, id).
		Scan(&result.ID, &result.TaskID, &result.SessionID, &result.State, &result.SchemaVersion, &result.BaseRepository, &result.BaseCommit, &result.WorktreePath, &result.Branch, &result.ResultBundle)
	if err != nil {
		respondError(w, 404, "NOT_FOUND", "run not found", nil)
		return
	}
	respondJSON(w, 200, map[string]any{"id": result.ID, "task_id": result.TaskID, "session_id": result.SessionID, "state": result.State, "schema_version": result.SchemaVersion, "base_repository": result.BaseRepository, "base_commit": result.BaseCommit, "worktree_path": result.WorktreePath, "branch": result.Branch, "result_bundle": result.ResultBundle})
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid run ID", nil)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	rows, err := s.pool.Query(r.Context(), `SELECT sequence,event_type,payload,created_at,schema_version FROM run_events WHERE run_id=$1 AND sequence>$2 ORDER BY sequence LIMIT 500`, id, after)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to replay events", nil)
		return
	}
	defer rows.Close()
	events := []map[string]any{}
	for rows.Next() {
		var sequence int64
		var eventType, version string
		var payload json.RawMessage
		var created time.Time
		if rows.Scan(&sequence, &eventType, &payload, &created, &version) == nil {
			events = append(events, map[string]any{"sequence": sequence, "type": eventType, "payload": payload, "time": created, "schema_version": version})
		}
	}
	respondJSON(w, 200, map[string]any{"events": events})
}

const runLeaseDuration = 30 * time.Second

type durableRun struct {
	ID           uuid.UUID
	LeaseOwner   string
	BaseRepo     string
	BaseCommit   string
	WorktreePath string
	Branch       string
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT id::text,task_id::text,session_id::text,state,schema_version,started_at,finished_at FROM task_runs ORDER BY started_at DESC LIMIT 100`)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to list runs", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, taskID, sessionID, state, version string
		var started time.Time
		var finished *time.Time
		if rows.Scan(&id, &taskID, &sessionID, &state, &version, &started, &finished) == nil {
			items = append(items, map[string]any{"id": id, "task_id": taskID, "session_id": sessionID, "state": state, "schema_version": version, "started_at": started, "finished_at": finished})
		}
	}
	respondJSON(w, 200, items)
}

func (s *Server) handleRunOutcome(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid run ID", nil)
		return
	}
	var req struct {
		Outcome string `json:"outcome"`
		Note    string `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Outcome != "accepted" && req.Outcome != "corrected" && req.Outcome != "rejected") {
		respondError(w, 400, "INVALID_REQUEST", "outcome must be accepted, corrected or rejected", nil)
		return
	}
	var worktree gitpkg.SessionWorktree
	var taskID, sessionID uuid.UUID
	var baseCommit string
	var resultBundle json.RawMessage
	err = s.pool.QueryRow(r.Context(), `SELECT task_id,session_id,base_repository,base_commit,worktree_path,branch,COALESCE(result_bundle,'{}') FROM task_runs WHERE id=$1 AND state='review' AND outcome IS NULL`, id).
		Scan(&taskID, &sessionID, &worktree.Repository, &baseCommit, &worktree.Path, &worktree.Branch, &resultBundle)
	if err != nil {
		respondError(w, 409, "INVALID_TRANSITION", "run is not awaiting an outcome", nil)
		return
	}
	var promotion *gitpkg.PromotionResult
	approval := gitpkg.Approval{Granted: true, Actor: "user:local", Reason: req.Note}
	switch req.Outcome {
	case "accepted":
		if gateErr := validatePromotionEvidence(worktree.Path, resultBundle); gateErr != nil {
			respondError(w, http.StatusConflict, "STALE_OR_UNVERIFIED_EVIDENCE", gateErr.Error(), map[string]any{
				"next_action": "ejecutá QA nuevamente sobre el diff actual antes de promover",
			})
			return
		}
		if readyErr := gitpkg.CheckPromotionReadiness(worktree, baseCommit); readyErr != nil {
			respondError(w, http.StatusConflict, "PROMOTION_NOT_READY", promotionReadinessMessage(readyErr), map[string]any{"next_action": "limpiá el checkout principal o actualizá el run antes de promover"})
			return
		}
		message := strings.TrimSpace(req.Note)
		if message == "" {
			message = "Accept oberth run " + id.String()
		}
		result, promoteErr := gitpkg.PromoteSessionWorktree(worktree, message, approval)
		verificationOnly := false
		if errors.Is(promoteErr, gitpkg.ErrNoChanges) {
			// A verification-only run has nothing to promote, but its successful
			// review is still a valid outcome. Remove the isolated branch and
			// record the acceptance without manufacturing an empty commit.
			if cleanupErr := gitpkg.CleanupSessionWorktree(worktree, false); cleanupErr != nil {
				respondError(w, http.StatusConflict, "PROMOTION_FAILED", cleanupErr.Error(), nil)
				return
			}
			promoteErr = nil
			verificationOnly = true
		}
		if promoteErr != nil {
			status := http.StatusConflict
			if errors.Is(promoteErr, gitpkg.ErrApprovalRequired) {
				status = http.StatusForbidden
			}
			respondError(w, status, "PROMOTION_FAILED", promoteErr.Error(), nil)
			return
		}
		if !verificationOnly {
			promotion = &result
			// Repository-scoped memories are evidence tied to a commit. Keep them
			// visible for audit, but remove stale entries from retrieval.
			_, _ = s.pool.Exec(r.Context(), `
			UPDATE memory_candidates
			SET status='expired', invalidated_at=NOW(),
			    invalidation_reason='repository advanced after accepted run',
			    validity_status='needs_revalidation'
			WHERE scope=$1 AND status='approved' AND source_commit<>$2
			  AND invalidated_at IS NULL`,
				worktree.Repository, result.Commit)
		}
	case "rejected":
		if cleanupErr := gitpkg.CleanupSessionWorktree(worktree, true); cleanupErr != nil {
			respondError(w, 409, "DISCARD_FAILED", cleanupErr.Error(), nil)
			return
		}
	case "corrected":
		// Corrections start a fresh governed run from the unchanged repository.
		// Discard the reviewed worktree so it cannot leak into the retry.
		if cleanupErr := gitpkg.CleanupSessionWorktree(worktree, true); cleanupErr != nil {
			respondError(w, 409, "DISCARD_FAILED", cleanupErr.Error(), nil)
			return
		}
	}
	tag, err := s.pool.Exec(r.Context(), `
		UPDATE task_runs
		SET outcome=$2,
		    outcome_at=NOW(),
		    state=CASE WHEN $2='rejected' THEN 'cancelled' WHEN $2 IN ('accepted','corrected') THEN 'completed' ELSE state END,
		    finished_at=CASE WHEN $2 IN ('accepted','corrected','rejected') THEN NOW() ELSE finished_at END,
		    result_bundle=jsonb_set(COALESCE(result_bundle,'{}'::jsonb),'{outcome}',to_jsonb($2::text),true),
		    approvals=approvals+CASE WHEN $2='accepted' THEN 1 ELSE 0 END,
		    interventions=interventions+CASE WHEN $2='corrected' THEN 1 ELSE 0 END
		WHERE id=$1 AND state='review' AND outcome IS NULL`, id, req.Outcome)
	if err != nil || tag.RowsAffected() != 1 {
		respondError(w, 409, "INVALID_TRANSITION", "run is not awaiting an outcome", nil)
		return
	}
	if req.Outcome == "accepted" || req.Outcome == "rejected" {
		taskState := "completed"
		sessionState := "completed"
		if req.Outcome == "rejected" {
			taskState, sessionState = "cancelled", "cancelled"
		}
		_, _ = s.pool.Exec(r.Context(), `UPDATE tasks SET status=$2, updated_at=NOW() WHERE id=$1 AND status='review'`, taskID, taskState)
		_, _ = s.pool.Exec(r.Context(), `UPDATE sessions SET status=$2, ended_at=COALESCE(ended_at,NOW()) WHERE id=$1 AND status='review'`, sessionID, sessionState)
	} else if req.Outcome == "corrected" {
		_, _ = s.pool.Exec(r.Context(), `UPDATE sessions SET status='completed', ended_at=COALESCE(ended_at,NOW()) WHERE id=$1 AND status='review'`, sessionID)
	}
	_ = s.appendRunEvent(r.Context(), id, "outcome_recorded", req)
	respondJSON(w, 200, map[string]any{"id": id, "outcome": req.Outcome, "promotion": promotion})
}

// reconcileResolvedRunLifecycle repairs installations created before outcomes
// advanced their parent task and session. It is intentionally idempotent so
// every daemon start can protect the UI from stale "review" records.
func (s *Server) reconcileResolvedRunLifecycle(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (task_id) task_id,outcome
			FROM task_runs
			WHERE outcome IS NOT NULL
			ORDER BY task_id,started_at DESC
		)
		UPDATE tasks AS task
		SET status=CASE latest.outcome WHEN 'accepted' THEN 'completed' WHEN 'rejected' THEN 'cancelled' ELSE task.status END,
		    updated_at=NOW()
		FROM latest
		WHERE task.id=latest.task_id
		  AND task.status='review'
		  AND latest.outcome IN ('accepted','rejected')`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions AS session
		SET status=CASE run.outcome WHEN 'rejected' THEN 'cancelled' ELSE 'completed' END,
		    ended_at=COALESCE(session.ended_at,run.outcome_at,NOW())
		FROM task_runs AS run
		WHERE session.id=run.session_id
		  AND session.status='review'
		  AND run.outcome IN ('accepted','corrected','rejected')`)
	return err
}

func (s *Server) handlePromotionReadiness(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid run ID", nil)
		return
	}
	var worktree gitpkg.SessionWorktree
	var baseCommit string
	var bundle json.RawMessage
	err = s.pool.QueryRow(r.Context(), `SELECT base_repository,base_commit,worktree_path,branch,COALESCE(result_bundle,'{}') FROM task_runs WHERE id=$1 AND state='review' AND outcome IS NULL`, id).Scan(&worktree.Repository, &baseCommit, &worktree.Path, &worktree.Branch, &bundle)
	if err != nil {
		respondError(w, 409, "INVALID_TRANSITION", "run is not awaiting an outcome", nil)
		return
	}
	if err := validatePromotionEvidence(worktree.Path, bundle); err != nil {
		respondJSON(w, 200, map[string]any{"ready": false, "reason": err.Error()})
		return
	}
	if err := gitpkg.CheckPromotionReadiness(worktree, baseCommit); err != nil {
		respondJSON(w, 200, map[string]any{"ready": false, "reason": promotionReadinessMessage(err)})
		return
	}
	respondJSON(w, 200, map[string]any{"ready": true})
}

func promotionReadinessMessage(err error) string {
	if errors.Is(err, gitpkg.ErrDirtyWorktree) {
		return "El checkout principal tiene cambios locales. Guardalos, confirmalos o descartalos antes de aplicar este run."
	}
	return err.Error()
}

func validatePromotionEvidence(worktreePath string, raw json.RawMessage) error {
	var evidence struct {
		DiffHash           string            `json:"diff_hash"`
		VerificationStatus string            `json:"verification_status"`
		Reasoning          *reasoning.CaseV1 `json:"reasoning"`
	}
	if json.Unmarshal(raw, &evidence) != nil {
		return errors.New("el result bundle no se puede validar")
	}
	if evidence.VerificationStatus != "passed" {
		return fmt.Errorf("la promoción requiere verificación aprobada; estado actual: %q", evidence.VerificationStatus)
	}
	currentDiff, err := gitpkg.GetDiff(worktreePath)
	if err != nil {
		return fmt.Errorf("no se pudo recalcular el diff actual: %w", err)
	}
	diffBytes, _ := json.Marshal(currentDiff)
	currentHash := fmt.Sprintf("sha256:%x", sha256.Sum256(diffBytes))
	if evidence.DiffHash == "" || evidence.DiffHash != currentHash {
		return fmt.Errorf("el diff cambió después de QA (verificado %s, actual %s)", evidence.DiffHash, currentHash)
	}
	if evidence.Reasoning != nil {
		refreshReasoningEvidence(worktreePath, currentHash, evidence.Reasoning)
		if len(evidence.Reasoning.Assessment.GateBlockers) > 0 {
			return fmt.Errorf("la promoción está bloqueada por evidencia epistemológica: %s",
				strings.Join(evidence.Reasoning.Assessment.GateBlockers, "; "))
		}
	}
	return nil
}

func (s *Server) handleOutcomeMetrics(w http.ResponseWriter, r *http.Request) {
	var total, accepted, greenAccepted int
	var avgFirstAction, avgVerification, costAccepted float64
	err := s.pool.QueryRow(r.Context(), `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE outcome='accepted'),
		       COUNT(*) FILTER (WHERE outcome='accepted' AND COALESCE(result_bundle->>'verification_status','')='passed'),
		       COALESCE(AVG(EXTRACT(EPOCH FROM (first_action_at-started_at))*1000) FILTER (WHERE first_action_at IS NOT NULL),0),
		       COALESCE(AVG(EXTRACT(EPOCH FROM (verification_at-started_at))*1000) FILTER (WHERE verification_at IS NOT NULL),0),
		       COALESCE(SUM((result_bundle->>'cost')::double precision) FILTER (WHERE outcome='accepted'),0)
		FROM task_runs`).Scan(&total, &accepted, &greenAccepted, &avgFirstAction, &avgVerification, &costAccepted)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to calculate outcome metrics", nil)
		return
	}
	acceptanceRate := 0.0
	if total > 0 {
		acceptanceRate = float64(accepted) * 100 / float64(total)
	}
	respondJSON(w, 200, map[string]any{"total_runs": total, "accepted": accepted, "accepted_with_green_tests": greenAccepted, "acceptance_rate": acceptanceRate, "avg_time_to_first_action_ms": avgFirstAction, "avg_time_to_verification_ms": avgVerification, "cost_per_accepted_task": costAccepted / float64(max(accepted, 1))})
}

func (s *Server) createDurableRun(ctx context.Context, taskID, sessionID uuid.UUID, baseRepo, baseCommit, worktree, branch, idempotencyKey string) (*durableRun, error) {
	run := &durableRun{
		ID: uuid.New(), LeaseOwner: hostname() + ":" + uuid.NewString(),
		BaseRepo: baseRepo, BaseCommit: baseCommit, WorktreePath: worktree, Branch: branch,
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO task_runs
			(id, task_id, session_id, state, lease_owner, lease_expires_at,
			 base_repository, base_commit, worktree_path, branch, idempotency_key)
		VALUES ($1,$2,$3,'running',$4,NOW()+$5::interval,$6,$7,$8,$9,NULLIF($10,''))`,
		run.ID, taskID, sessionID, run.LeaseOwner, runLeaseDuration.String(),
		baseRepo, baseCommit, worktree, branch, idempotencyKey)
	if err != nil {
		return nil, err
	}
	_ = s.appendRunEvent(ctx, run.ID, "run_started", map[string]any{"worktree": worktree, "branch": branch, "base_commit": baseCommit})
	return run, nil
}

func (s *Server) heartbeatRun(ctx context.Context, run *durableRun) bool {
	tag, err := s.pool.Exec(ctx, `
		UPDATE task_runs
		SET heartbeat_at=NOW(), lease_expires_at=NOW()+$3::interval, version=version+1
		WHERE id=$1 AND lease_owner=$2 AND state='running'`,
		run.ID, run.LeaseOwner, runLeaseDuration.String())
	return err == nil && tag.RowsAffected() == 1
}

func (s *Server) finishDurableRun(ctx context.Context, runID uuid.UUID, state string, bundle any) {
	data, _ := secretspkg.MarshalRedacted(bundle)
	_, _ = s.pool.Exec(ctx, `
		UPDATE task_runs SET state=$2, result_bundle=$3, finished_at=NOW(),
			lease_expires_at=NOW(), version=version+1
		WHERE id=$1 AND state='running'`, runID, state, data)
	_ = s.appendRunEvent(ctx, runID, "run_"+state, bundle)
}

func (s *Server) appendRunEvent(ctx context.Context, runID uuid.UUID, eventType string, payload any) error {
	data, _ := secretspkg.MarshalRedacted(payload)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO run_events (run_id, sequence, event_type, payload)
		SELECT $1, COALESCE(MAX(sequence),0)+1, $2, $3
		FROM run_events WHERE run_id=$1`, runID, eventType, data)
	if err == nil && eventType == "agent_turn" {
		_, _ = s.pool.Exec(ctx, `UPDATE task_runs SET first_action_at=COALESCE(first_action_at,NOW()) WHERE id=$1`, runID)
	}
	return err
}

func (s *Server) ReconcileInterruptedRuns(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		WITH interrupted AS (
			UPDATE task_runs SET state='interrupted', finished_at=NOW(), version=version+1
			WHERE state='running' AND lease_expires_at < NOW()
			RETURNING task_id, session_id
		)
		UPDATE tasks SET status='blocked', updated_at=NOW()
		WHERE id IN (SELECT task_id FROM interrupted) AND status='running'`)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE sessions SET status='blocked', ended_at=NOW()
		WHERE id IN (SELECT session_id FROM task_runs WHERE state='interrupted')
		  AND status='active'`)
	return err
}

func (s *Server) ReconcileTerminalWorktrees(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT base_repository,worktree_path,branch
		FROM task_runs
		WHERE state IN ('cancelled','failed')
		   OR outcome='rejected'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var cleanupErr error
	for rows.Next() {
		var worktree gitpkg.SessionWorktree
		if err := rows.Scan(&worktree.Repository, &worktree.Path, &worktree.Branch); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		}
		if err := gitpkg.CleanupSessionWorktree(worktree, true); err != nil &&
			!errors.Is(err, os.ErrNotExist) && !strings.Contains(strings.ToLower(err.Error()), "not a working tree") {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return errors.Join(cleanupErr, rows.Err())
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "oberth"
	}
	return name
}
