ALTER TABLE task_runs
    ADD COLUMN resume_from_sequence BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN recovery_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN cancel_requested_at TIMESTAMPTZ;

CREATE INDEX idx_task_runs_recovery
    ON task_runs(state, lease_expires_at, resume_from_sequence);
