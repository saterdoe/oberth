DROP INDEX IF EXISTS idx_task_runs_idempotency;
ALTER TABLE task_runs DROP COLUMN IF EXISTS idempotency_key;
