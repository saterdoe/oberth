ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_status_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_status_check
  CHECK (status IN ('active', 'completed', 'cancelled', 'error', 'approved',
                    'changes_requested', 'rejected', 'pending', 'review', 'blocked'));

CREATE TABLE approval_resolutions (
    id UUID PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    schema_version TEXT NOT NULL DEFAULT '1',
    scope TEXT NOT NULL CHECK (scope IN ('once','session','project')),
    decision TEXT NOT NULL CHECK (decision IN ('allow','deny')),
    operation TEXT NOT NULL,
    target TEXT NOT NULL,
    user_id TEXT NOT NULL,
    task_id UUID,
    session_id UUID,
    run_id UUID,
    repository_path TEXT NOT NULL,
    risk TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX idx_approval_resolutions_lookup
  ON approval_resolutions(scope, session_id, repository_path, operation, created_at DESC);
