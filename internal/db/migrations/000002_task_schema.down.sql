DROP TABLE IF EXISTS context_logs;
DROP TABLE IF EXISTS approval_gates;

ALTER TABLE sessions
  DROP COLUMN IF EXISTS branch,
  DROP COLUMN IF EXISTS plan,
  DROP COLUMN IF EXISTS diff_summary,
  DROP COLUMN IF EXISTS artifacts,
  DROP COLUMN IF EXISTS approved_by,
  DROP COLUMN IF EXISTS approved_at,
  DROP COLUMN IF EXISTS context_used,
  DROP COLUMN IF EXISTS risk_score,
  DROP COLUMN IF EXISTS approval_required;
