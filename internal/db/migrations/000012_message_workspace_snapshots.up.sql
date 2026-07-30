CREATE TABLE message_workspace_snapshots (
    message_id UUID PRIMARY KEY REFERENCES chat_messages(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    repo_path TEXT NOT NULL,
    head_commit TEXT NOT NULL DEFAULT '',
    git_branch TEXT NOT NULL DEFAULT '',
    is_clean BOOLEAN NOT NULL DEFAULT TRUE,
    diff_patch TEXT NOT NULL DEFAULT '',
    working_files JSONB NOT NULL DEFAULT '[]'::jsonb,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_message_workspace_snapshots_session
    ON message_workspace_snapshots(session_id, captured_at);
