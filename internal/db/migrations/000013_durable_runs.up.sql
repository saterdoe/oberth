CREATE TABLE task_runs (
    id UUID PRIMARY KEY,
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    schema_version TEXT NOT NULL DEFAULT '1',
    state TEXT NOT NULL CHECK (state IN ('running','review','completed','blocked','cancelled','failed','interrupted')),
    lease_owner TEXT NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    base_repository TEXT NOT NULL,
    base_commit TEXT NOT NULL,
    worktree_path TEXT NOT NULL,
    branch TEXT NOT NULL,
    result_bundle JSONB,
    version BIGINT NOT NULL DEFAULT 1,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE TABLE run_events (
    id BIGSERIAL PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES task_runs(id) ON DELETE CASCADE,
    schema_version TEXT NOT NULL DEFAULT '1',
    sequence BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, sequence)
);

CREATE INDEX idx_task_runs_state_lease ON task_runs(state, lease_expires_at);
CREATE INDEX idx_run_events_run_sequence ON run_events(run_id, sequence);
