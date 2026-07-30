ALTER TABLE task_runs ADD COLUMN idempotency_key TEXT;

CREATE UNIQUE INDEX idx_task_runs_idempotency
    ON task_runs(task_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
