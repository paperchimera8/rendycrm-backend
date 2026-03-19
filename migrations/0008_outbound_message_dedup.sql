ALTER TABLE outbound_messages
    ADD COLUMN IF NOT EXISTS dedup_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_outbound_messages_dedup_key
    ON outbound_messages (dedup_key)
    WHERE dedup_key <> '';
