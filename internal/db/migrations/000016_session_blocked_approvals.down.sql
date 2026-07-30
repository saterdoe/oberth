DROP TABLE IF EXISTS approval_resolutions;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_status_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_status_check
  CHECK (status IN ('active', 'completed', 'cancelled', 'error', 'approved',
                    'changes_requested', 'rejected', 'pending', 'review'));
