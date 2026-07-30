CREATE TABLE session_branches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    parent_branch_id UUID REFERENCES session_branches(id) ON DELETE SET NULL,
    forked_from_message_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE chat_messages
    ADD COLUMN branch_id UUID REFERENCES session_branches(id) ON DELETE CASCADE,
    ADD COLUMN parent_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL;

CREATE INDEX idx_session_branches_session ON session_branches(session_id, created_at);
CREATE INDEX idx_chat_messages_branch ON chat_messages(branch_id, created_at);
