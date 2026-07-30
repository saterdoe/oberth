ALTER TABLE task_runs
    DROP COLUMN IF EXISTS outcome_at,
    DROP COLUMN IF EXISTS approvals,
    DROP COLUMN IF EXISTS interventions,
    DROP COLUMN IF EXISTS outcome,
    DROP COLUMN IF EXISTS verification_at,
    DROP COLUMN IF EXISTS first_action_at;
