DROP INDEX IF EXISTS idx_execution_logs_session;
DROP INDEX IF EXISTS idx_audit_log_actor;
DROP INDEX IF EXISTS idx_audit_log_action;
DROP INDEX IF EXISTS idx_audit_log_created_at;
DROP INDEX IF EXISTS idx_cost_logs_session;
DROP INDEX IF EXISTS idx_cost_logs_created_at;
DROP INDEX IF EXISTS idx_sessions_status;
DROP INDEX IF EXISTS idx_sessions_user_id;
DROP INDEX IF EXISTS idx_sessions_started_at;

DROP TABLE IF EXISTS execution_logs;
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS budgets;
DROP TABLE IF EXISTS cost_logs;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS routing_rules;
DROP TABLE IF EXISTS providers;
DROP TABLE IF EXISTS users;

DROP EXTENSION IF EXISTS "pgcrypto";
