WITH latest AS (
    SELECT DISTINCT ON (task_id) task_id, outcome
    FROM task_runs
    WHERE outcome IS NOT NULL
    ORDER BY task_id, started_at DESC
)
UPDATE tasks AS task
SET status = CASE latest.outcome
        WHEN 'accepted' THEN 'completed'
        WHEN 'rejected' THEN 'cancelled'
        ELSE task.status
    END,
    updated_at = NOW()
FROM latest
WHERE task.id = latest.task_id
  AND task.status = 'review'
  AND latest.outcome IN ('accepted', 'rejected');

UPDATE sessions AS session
SET status = CASE run.outcome
        WHEN 'rejected' THEN 'cancelled'
        ELSE 'completed'
    END,
    ended_at = COALESCE(session.ended_at, run.outcome_at, NOW())
FROM task_runs AS run
WHERE session.id = run.session_id
  AND session.status = 'review'
  AND run.outcome IN ('accepted', 'corrected', 'rejected');
