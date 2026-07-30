DROP INDEX IF EXISTS idx_chat_messages_branch;
DROP INDEX IF EXISTS idx_session_branches_session;
ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS parent_message_id,
    DROP COLUMN IF EXISTS branch_id;
DROP TABLE IF EXISTS session_branches;
