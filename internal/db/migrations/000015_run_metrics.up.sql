ALTER TABLE task_runs
    ADD COLUMN first_action_at TIMESTAMPTZ,
    ADD COLUMN verification_at TIMESTAMPTZ,
    ADD COLUMN outcome TEXT CHECK (outcome IN ('accepted','corrected','rejected')),
    ADD COLUMN interventions INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN approvals INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN outcome_at TIMESTAMPTZ;
