DROP INDEX IF EXISTS idx_task_runs_recovery;

ALTER TABLE task_runs
    DROP COLUMN IF EXISTS cancel_requested_at,
    DROP COLUMN IF EXISTS recovery_count,
    DROP COLUMN IF EXISTS resume_from_sequence;
