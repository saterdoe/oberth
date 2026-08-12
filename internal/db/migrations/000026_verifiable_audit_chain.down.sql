ALTER TABLE audit_log
    DROP CONSTRAINT IF EXISTS audit_log_entry_hash_unique,
    DROP CONSTRAINT IF EXISTS audit_log_sequence_unique,
    DROP COLUMN IF EXISTS entry_hash,
    DROP COLUMN IF EXISTS prev_hash,
    DROP COLUMN IF EXISTS sequence,
    DROP COLUMN IF EXISTS decision,
    DROP COLUMN IF EXISTS target,
    DROP COLUMN IF EXISTS correlation_id;
