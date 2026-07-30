CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user', 'readonly')),
    password_hash TEXT,
    sso_provider TEXT,
    sso_external_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('openai','anthropic','google','ollama','vllm','tgi','custom')),
    base_url TEXT,
    api_key_encrypted TEXT,
    default_model TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    priority INT NOT NULL DEFAULT 0,
    rate_limit_rpm INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE routing_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    priority INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    match_repo_pattern TEXT,
    match_task_type TEXT,
    match_user_id UUID REFERENCES users(id),
    provider_id UUID NOT NULL REFERENCES providers(id),
    model TEXT NOT NULL,
    execution_graph JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    repo_path TEXT,
    task_type TEXT NOT NULL,
    task_description TEXT,
    provider_id UUID REFERENCES providers(id),
    model TEXT,
    tokens_input INT NOT NULL DEFAULT 0,
    tokens_output INT NOT NULL DEFAULT 0,
    cost NUMERIC(12,6) NOT NULL DEFAULT 0,
    duration_ms INT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','completed','cancelled','error')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ
);

CREATE TABLE cost_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID REFERENCES sessions(id) ON DELETE CASCADE,
    provider_id UUID REFERENCES providers(id),
    model TEXT NOT NULL,
    tokens_input INT NOT NULL,
    tokens_output INT NOT NULL,
    cost_input NUMERIC(12,6) NOT NULL,
    cost_output NUMERIC(12,6) NOT NULL,
    cost_total NUMERIC(12,6) NOT NULL,
    cache_hit BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    provider_id UUID REFERENCES providers(id),
    soft_limit NUMERIC(12,2) NOT NULL,
    hard_limit NUMERIC(12,2) NOT NULL,
    period TEXT NOT NULL DEFAULT 'monthly' CHECK (period IN ('daily','weekly','monthly','quarterly','yearly')),
    current_spend NUMERIC(12,2) NOT NULL DEFAULT 0,
    period_start TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID REFERENCES sessions(id),
    action TEXT NOT NULL,
    actor TEXT NOT NULL CHECK (actor LIKE 'agent:%' OR actor LIKE 'user:%'),
    details JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE execution_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID REFERENCES sessions(id),
    step_id TEXT NOT NULL,
    provider_id UUID REFERENCES providers(id),
    model TEXT NOT NULL,
    parent_step TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','success','failed','skipped')),
    tokens_input INT NOT NULL DEFAULT 0,
    tokens_output INT NOT NULL DEFAULT 0,
    cost NUMERIC(12,6) NOT NULL DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_sessions_started_at ON sessions(started_at DESC);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_cost_logs_created_at ON cost_logs(created_at DESC);
CREATE INDEX idx_cost_logs_session ON cost_logs(session_id);
CREATE INDEX idx_audit_log_created_at ON audit_log(created_at DESC);
CREATE INDEX idx_audit_log_action ON audit_log(action);
CREATE INDEX idx_audit_log_actor ON audit_log(actor);
CREATE INDEX idx_execution_logs_session ON execution_logs(session_id);
