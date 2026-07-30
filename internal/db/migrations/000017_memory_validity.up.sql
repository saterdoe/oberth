ALTER TABLE memory_candidates
  ADD COLUMN content_hash TEXT,
  ADD COLUMN supersedes UUID REFERENCES memory_candidates(id),
  ADD COLUMN invalidated_at TIMESTAMPTZ,
  ADD COLUMN invalidation_reason TEXT;

UPDATE memory_candidates
SET content_hash = md5(lower(regexp_replace(trim(content), '\s+', ' ', 'g')))
WHERE content_hash IS NULL;

ALTER TABLE memory_candidates ALTER COLUMN content_hash SET NOT NULL;
CREATE UNIQUE INDEX idx_memory_candidate_pending_dedupe
  ON memory_candidates(scope, kind, content_hash)
  WHERE status IN ('pending','approved') AND invalidated_at IS NULL;
CREATE INDEX idx_memory_candidate_source_commit ON memory_candidates(scope, source_commit);
