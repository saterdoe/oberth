CREATE TABLE memory_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schema_version TEXT NOT NULL DEFAULT '1',
    run_id UUID REFERENCES task_runs(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('fact','decision','preference','summary')),
    content TEXT NOT NULL,
    source_commit TEXT NOT NULL,
    created_by TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    scope TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','expired')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_at TIMESTAMPTZ
);
CREATE INDEX idx_memory_candidates_status ON memory_candidates(status, created_at DESC);
