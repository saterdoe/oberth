UPDATE audit_log SET actor = 'agent:system' WHERE actor = 'system';

ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_actor_check;

ALTER TABLE audit_log
    ADD CONSTRAINT audit_log_actor_check
    CHECK (actor LIKE 'agent:%' OR actor LIKE 'user:%');
