-- Expand sessions table with task workflow columns
ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS branch       TEXT,
  ADD COLUMN IF NOT EXISTS plan         JSONB,
  ADD COLUMN IF NOT EXISTS diff_summary TEXT,
  ADD COLUMN IF NOT EXISTS artifacts    JSONB DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS approved_by  UUID REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS approved_at  TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS context_used JSONB DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS risk_score   TEXT CHECK (risk_score IN ('low','medium','high','critical')) DEFAULT 'low',
  ADD COLUMN IF NOT EXISTS approval_required BOOLEAN DEFAULT false;

-- Approval gates table
CREATE TABLE IF NOT EXISTS approval_gates (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name          TEXT NOT NULL,
  description   TEXT,
  repo_pattern  TEXT,
  task_type     TEXT,
  provider_id   UUID REFERENCES providers(id),
  require_approval BOOLEAN NOT NULL DEFAULT true,
  require_review   BOOLEAN NOT NULL DEFAULT false,
  deny_cloud       BOOLEAN NOT NULL DEFAULT false,
  require_tests    BOOLEAN NOT NULL DEFAULT false,
  max_cost         NUMERIC(12,6),
  priority         INT NOT NULL DEFAULT 0,
  is_active        BOOLEAN NOT NULL DEFAULT true,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Context log table
CREATE TABLE IF NOT EXISTS context_logs (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id    UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  source_type   TEXT NOT NULL,  -- vault_note, source_file, adr, etc
  source_path   TEXT NOT NULL,
  relevance     REAL DEFAULT 0,
  tokens_approx INT DEFAULT 0,
  sent_to_cloud BOOLEAN NOT NULL DEFAULT false,
  excluded      BOOLEAN NOT NULL DEFAULT false,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_context_logs_session ON context_logs(session_id);
CREATE INDEX IF NOT EXISTS idx_approval_gates_priority ON approval_gates(priority, is_active);
