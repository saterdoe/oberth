package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	semcontext "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/reasoning"
	gitpkg "github.com/saterdoe/oberth/pkg/git"
	secretspkg "github.com/saterdoe/oberth/pkg/secrets"
)

type memoryProposal struct {
	Kind, ClaimID, Content string
	EvidenceIDs            []string
	Confidence             float64
}

func reasoningMemoryProposals(current *reasoning.CaseV1) []memoryProposal {
	if current == nil {
		return nil
	}
	result := []memoryProposal{}
	for _, record := range current.Records {
		if (record.Kind != reasoning.KindFact && record.Kind != reasoning.KindDecision && record.Kind != reasoning.KindProperty) ||
			(record.Status != reasoning.StatusSupported && record.Status != reasoning.StatusPassed) ||
			len(record.EvidenceIDs) == 0 {
			continue
		}
		confidence := .8
		if record.Confidence != nil {
			confidence = *record.Confidence
		}
		result = append(result, memoryProposal{
			Kind: string(record.Kind), ClaimID: record.ID, Content: record.Statement,
			EvidenceIDs: record.EvidenceIDs, Confidence: confidence,
		})
	}
	for _, experiment := range current.Experiments {
		if experiment.Status != reasoning.StatusPassed || len(experiment.EvidenceIDs) == 0 {
			continue
		}
		result = append(result, memoryProposal{
			Kind: "experiment", ClaimID: experiment.ID,
			Content:     fmt.Sprintf("%s — %s => %s", experiment.Question, experiment.Command, experiment.Observation),
			EvidenceIDs: experiment.EvidenceIDs, Confidence: .9,
		})
	}
	return result
}

func verifiedRunMemoryProposal(runID uuid.UUID, title string, current *reasoning.CaseV1, diff []gitpkg.DiffFile) *memoryProposal {
	if current == nil || len(diff) == 0 {
		return nil
	}
	evidenceIDs := []string{}
	for _, evidence := range current.Evidence {
		if !evidence.Stale && strings.HasPrefix(evidence.Source, "command:") {
			evidenceIDs = append(evidenceIDs, evidence.ID)
		}
	}
	if len(evidenceIDs) == 0 {
		return nil
	}
	paths := make([]string, 0, len(diff))
	for _, file := range diff {
		paths = append(paths, file.Path)
	}
	return &memoryProposal{
		Kind: "summary", ClaimID: "verified-run:" + runID.String(),
		Content: fmt.Sprintf("Verified change for %q affects %s; repository verification passed in the isolated worktree.",
			strings.TrimSpace(title), strings.Join(paths, ", ")),
		EvidenceIDs: evidenceIDs, Confidence: .7,
	}
}

func (s *Server) createMemoryCandidates(ctx context.Context, run *durableRun, current *reasoning.CaseV1, summary string, fallback *memoryProposal) {
	proposals := reasoningMemoryProposals(current)
	if len(proposals) == 0 {
		if strings.TrimSpace(summary) != "" {
			proposal := memoryProposal{Kind: "summary", Content: summary, Confidence: .6}
			if fallback != nil {
				proposal.ClaimID = fallback.ClaimID
				proposal.EvidenceIDs = fallback.EvidenceIDs
			}
			proposals = []memoryProposal{proposal}
		} else if fallback != nil {
			proposals = []memoryProposal{*fallback}
		}
	}
	for _, proposal := range proposals {
		s.createMemoryCandidate(ctx, run, proposal)
	}
}

func (s *Server) createMemoryCandidate(ctx context.Context, run *durableRun, proposal memoryProposal) {
	content := strings.TrimSpace(proposal.Content)
	if content == "" {
		return
	}
	normalized := strings.Join(strings.Fields(strings.ToLower(content)), " ")
	contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
	var supersedes *uuid.UUID
	rows, err := s.pool.Query(ctx, `SELECT id,content FROM memory_candidates WHERE scope=$1 AND kind=$2 AND status='approved' AND invalidated_at IS NULL ORDER BY created_at DESC LIMIT 50`, run.BaseRepo, proposal.Kind)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			var existing string
			if rows.Scan(&id, &existing) == nil && memoryContradicts(existing, content) {
				supersedes = &id
				break
			}
		}
	}
	evidenceJSON, _ := json.Marshal(proposal.EvidenceIDs)
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO memory_candidates (run_id,kind,claim_id,content,content_hash,source_commit,created_by,confidence,scope,expires_at,supersedes,contradicts,evidence_ids,validity_status)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,'agent:typed-runtime',
		        CASE WHEN $9::uuid IS NULL THEN $7 ELSE LEAST($7,0.5) END,
		        $8,NOW()+INTERVAL '30 days',$9,$9,$10,'current')
		ON CONFLICT (scope,kind,content_hash)
		  WHERE status IN ('pending','approved') AND invalidated_at IS NULL
		DO NOTHING`,
		run.ID, proposal.Kind, proposal.ClaimID, content, contentHash, run.BaseCommit,
		proposal.Confidence, run.BaseRepo, supersedes, evidenceJSON)
}

func memoryContradicts(left, right string) bool {
	normalize := func(value string) (map[string]bool, bool) {
		lower := strings.ToLower(value)
		negated := strings.Contains(lower, " not ") || strings.Contains(lower, " no ") ||
			strings.Contains(lower, "never") || strings.Contains(lower, "avoid") ||
			strings.Contains(lower, "do not") || strings.Contains(lower, "don't")
		for _, marker := range []string{" not ", " no ", "never", "avoid", "do not", "don't"} {
			lower = strings.ReplaceAll(lower, marker, " ")
		}
		words := map[string]bool{}
		for _, word := range strings.Fields(lower) {
			word = strings.Trim(word, ".,:;!?()[]{}\"'")
			if len(word) >= 4 {
				words[word] = true
			}
		}
		return words, negated
	}
	a, negA := normalize(left)
	b, negB := normalize(right)
	if negA == negB || len(a) == 0 || len(b) == 0 {
		return false
	}
	common := 0
	for word := range a {
		if b[word] {
			common++
		}
	}
	denominator := min(len(a), len(b))
	return denominator > 0 && float64(common)/float64(denominator) >= 0.6
}

func (s *Server) handleListMemoryCandidates(w http.ResponseWriter, r *http.Request) {
	_, _ = s.pool.Exec(r.Context(), `
		UPDATE memory_candidates SET status='expired',invalidated_at=COALESCE(invalidated_at,NOW()),
		       invalidation_reason=COALESCE(invalidation_reason,'candidate expired')
		WHERE expires_at<NOW() AND status IN ('pending','approved')`)
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	rows, err := s.pool.Query(r.Context(), `SELECT id::text,schema_version,kind,content,source_commit,created_by,confidence,scope,expires_at,status,created_at,
		COALESCE(claim_id,''),evidence_ids,validity_status,COALESCE(supersedes::text,''),COALESCE(contradicts::text,'')
		FROM memory_candidates WHERE status=$1 ORDER BY created_at DESC LIMIT 200`, status)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "failed to list memory candidates", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, version, kind, content, commit, createdBy, scope, state, claimID, validity, supersedes, contradicts string
		var confidence float64
		var expires any
		var created any
		var evidenceIDs json.RawMessage
		if rows.Scan(&id, &version, &kind, &content, &commit, &createdBy, &confidence, &scope, &expires, &state, &created, &claimID, &evidenceIDs, &validity, &supersedes, &contradicts) == nil {
			items = append(items, map[string]any{"id": id, "schema_version": version, "kind": kind, "claim_id": claimID, "content": content, "source_commit": commit, "created_by": createdBy, "confidence": confidence, "scope": scope, "expires_at": expires, "status": state, "created_at": created, "evidence_ids": evidenceIDs, "validity_status": validity, "supersedes": supersedes, "contradicts": contradicts})
		}
	}
	respondJSON(w, 200, items)
}

func (s *Server) handleDecideMemoryCandidate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, 400, "INVALID_ID", "invalid candidate ID", nil)
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Content  string `json:"content,omitempty"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || (req.Decision != "approved" && req.Decision != "rejected") {
		respondError(w, 400, "INVALID_REQUEST", "decision must be approved or rejected", nil)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE memory_candidates SET status=$2,content=COALESCE(NULLIF($3,''),content),decided_at=NOW() WHERE id=$1 AND status='pending'`, id, req.Decision, strings.TrimSpace(req.Content))
	if err != nil || tag.RowsAffected() != 1 {
		respondError(w, 409, "INVALID_TRANSITION", "candidate was already decided", nil)
		return
	}
	result := map[string]any{"id": id, "status": req.Decision}
	if req.Decision == "approved" {
		rollbackApproval := func() {
			_, _ = s.pool.Exec(context.Background(), `UPDATE memory_candidates SET status='pending',decided_at=NULL WHERE id=$1 AND status='approved'`, id)
		}
		var content, scope, sourceCommit, createdBy, kind, claimID, supersedes string
		var confidence float64
		var evidenceIDs json.RawMessage
		err = s.pool.QueryRow(r.Context(), `SELECT content,scope,source_commit,created_by,confidence,kind,COALESCE(claim_id,''),evidence_ids,COALESCE(supersedes::text,'') FROM memory_candidates WHERE id=$1`, id).
			Scan(&content, &scope, &sourceCommit, &createdBy, &confidence, &kind, &claimID, &evidenceIDs, &supersedes)
		if err != nil {
			rollbackApproval()
			respondError(w, 500, "MEMORY_PROMOTION_FAILED", "candidate approved but could not be loaded", nil)
			return
		}
		scopeHash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(scope)))))[:12]
		notePath := fmt.Sprintf("projects/%s/sessions/%s", scopeHash, id.String())
		metadata := map[string]any{
			"type": "session", "knowledge_kind": kind, "date": time.Now().UTC().Format(time.RFC3339),
			"scope": scope, "source_commit": sourceCommit, "created_by": createdBy,
			"confidence": confidence, "candidate_id": id.String(), "claim_id": claimID,
			"evidence_ids": evidenceIDs, "validity_status": "current", "supersedes": supersedes,
		}
		if _, err = s.vaultConn.UpsertNote(notePath, secretspkg.Redact(content), metadata); err != nil {
			rollbackApproval()
			respondError(w, 500, "MEMORY_PROMOTION_FAILED", err.Error(), nil)
			return
		}
		notes, listErr := s.vaultConn.ListAllNotes()
		if listErr != nil {
			_ = s.vaultConn.DeleteNote(notePath)
			rollbackApproval()
			respondError(w, 500, "MEMORY_PROMOTION_FAILED", "failed to rebuild memory index", nil)
			return
		}
		filtered := notes[:0]
		supersededNotePath := ""
		if supersedes != "" {
			supersededNotePath = fmt.Sprintf("projects/%s/sessions/%s", scopeHash, supersedes)
		}
		for _, note := range notes {
			if note.Path != "memory-index" && note.Path != supersededNotePath {
				filtered = append(filtered, note)
			}
		}
		if _, err = s.vaultConn.UpsertNote("memory-index", semcontext.BuildMemoryIndex(filtered), map[string]any{
			"type": "memory_index", "date": time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			_ = s.vaultConn.DeleteNote(notePath)
			rollbackApproval()
			respondError(w, 500, "MEMORY_PROMOTION_FAILED", "failed to persist memory index", nil)
			return
		}
		if s.indexer != nil {
			go func() { _, _ = s.indexer.ReindexIncremental(context.Background()) }()
		}
		if supersedes != "" {
			_, _ = s.pool.Exec(r.Context(), `UPDATE memory_candidates
				SET validity_status='superseded',invalidated_at=COALESCE(invalidated_at,NOW()),
				    invalidation_reason='superseded by approved reasoning memory',status='expired'
				WHERE id=$1 AND status='approved'`, supersedes)
			_ = s.vaultConn.DeleteNote(supersededNotePath)
		}
		s.contextCache.Clear()
		s.broadcastEvent(Event{Type: EventVaultChange, AggregateID: notePath, Payload: map[string]any{"path": notePath, "action": "promoted"}})
		result["note_path"] = notePath
	}
	respondJSON(w, 200, result)
}
