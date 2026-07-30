DROP INDEX IF EXISTS idx_memory_candidate_claim;
ALTER TABLE memory_candidates
  DROP COLUMN IF EXISTS contradicts,
  DROP COLUMN IF EXISTS validity_status,
  DROP COLUMN IF EXISTS evidence_ids,
  DROP COLUMN IF EXISTS claim_id,
  DROP CONSTRAINT IF EXISTS memory_candidates_kind_check,
  ADD CONSTRAINT memory_candidates_kind_check
    CHECK (kind IN ('fact','decision','preference','summary'));
