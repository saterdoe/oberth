ALTER TABLE memory_candidates
  DROP CONSTRAINT IF EXISTS memory_candidates_kind_check,
  ADD CONSTRAINT memory_candidates_kind_check
    CHECK (kind IN ('fact','decision','preference','summary','property','experiment')),
  ADD COLUMN claim_id TEXT,
  ADD COLUMN evidence_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN validity_status TEXT NOT NULL DEFAULT 'current'
    CHECK (validity_status IN ('current','needs_revalidation','contradicted','superseded')),
  ADD COLUMN contradicts UUID REFERENCES memory_candidates(id);

CREATE INDEX idx_memory_candidate_claim
  ON memory_candidates(scope, claim_id)
  WHERE claim_id IS NOT NULL;

