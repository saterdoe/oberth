DROP INDEX IF EXISTS idx_memory_candidate_source_commit;
DROP INDEX IF EXISTS idx_memory_candidate_pending_dedupe;
ALTER TABLE memory_candidates
  DROP COLUMN IF EXISTS invalidation_reason,
  DROP COLUMN IF EXISTS invalidated_at,
  DROP COLUMN IF EXISTS supersedes,
  DROP COLUMN IF EXISTS content_hash;
