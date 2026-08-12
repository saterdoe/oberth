ALTER TABLE audit_log
    ADD COLUMN correlation_id UUID,
    ADD COLUMN target TEXT,
    ADD COLUMN decision TEXT,
    ADD COLUMN sequence BIGINT,
    ADD COLUMN prev_hash TEXT,
    ADD COLUMN entry_hash TEXT;

DO $$
DECLARE
    item RECORD;
    previous TEXT := '';
    next_sequence BIGINT := 0;
    correlation UUID;
    computed TEXT;
BEGIN
    FOR item IN SELECT id,session_id,action,actor,details FROM audit_log ORDER BY created_at,id LOOP
        next_sequence := next_sequence + 1;
        correlation := gen_random_uuid();
        computed := encode(digest(concat_ws('|',previous,COALESCE(item.session_id::text,''),correlation::text,
            item.action,item.actor,'unspecified','recorded',item.details::text),'sha256'),'hex');
        UPDATE audit_log SET correlation_id=correlation,target='unspecified',decision='recorded',
            sequence=next_sequence,prev_hash=previous,entry_hash=computed WHERE id=item.id;
        previous := computed;
    END LOOP;
END $$;

ALTER TABLE audit_log
    ALTER COLUMN correlation_id SET NOT NULL,
    ALTER COLUMN target SET NOT NULL,
    ALTER COLUMN decision SET NOT NULL,
    ALTER COLUMN sequence SET NOT NULL,
    ALTER COLUMN prev_hash SET NOT NULL,
    ALTER COLUMN entry_hash SET NOT NULL,
    ADD CONSTRAINT audit_log_sequence_unique UNIQUE(sequence),
    ADD CONSTRAINT audit_log_entry_hash_unique UNIQUE(entry_hash);
