ALTER TABLE channel_accounts
    ADD COLUMN IF NOT EXISTS channel_kind TEXT,
    ADD COLUMN IF NOT EXISTS bot_username TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS bot_token_encrypted TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

UPDATE channel_accounts
SET channel_kind = CASE
    WHEN provider = 'telegram' AND account_name ILIKE '%operator%' THEN 'telegram_operator'
    WHEN provider = 'telegram' THEN 'telegram_client'
    WHEN provider = 'whatsapp' THEN 'whatsapp_twilio'
    ELSE provider
END
WHERE COALESCE(channel_kind, '') = '';

ALTER TABLE channel_accounts
    ALTER COLUMN channel_kind SET NOT NULL;

ALTER TABLE channel_accounts
    DROP CONSTRAINT IF EXISTS channel_accounts_workspace_id_provider_key;

ALTER TABLE channel_accounts
    DROP CONSTRAINT IF EXISTS channel_accounts_channel_kind_check;

ALTER TABLE channel_accounts
    ADD CONSTRAINT channel_accounts_channel_kind_check
    CHECK (channel_kind IN ('telegram_client', 'telegram_operator', 'whatsapp_twilio'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_accounts_workspace_kind_unique
    ON channel_accounts (workspace_id, channel_kind)
    WHERE revoked_at IS NULL;

ALTER TABLE bot_sessions
    ADD COLUMN IF NOT EXISTS bot_kind TEXT NOT NULL DEFAULT 'telegram_operator';

ALTER TABLE bot_sessions
    DROP CONSTRAINT IF EXISTS bot_sessions_bot_kind_check;

ALTER TABLE bot_sessions
    ADD CONSTRAINT bot_sessions_bot_kind_check
    CHECK (bot_kind IN ('telegram_client', 'telegram_operator', 'whatsapp_twilio'));

CREATE INDEX IF NOT EXISTS idx_bot_sessions_workspace_kind_actor
    ON bot_sessions (workspace_id, bot_kind, scope, actor_type, actor_id);

CREATE TABLE IF NOT EXISTS telegram_update_dedup (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_account_id TEXT REFERENCES channel_accounts(id) ON DELETE CASCADE,
    bot_kind TEXT NOT NULL,
    update_id BIGINT NOT NULL DEFAULT 0,
    chat_id TEXT NOT NULL DEFAULT '',
    message_id BIGINT NOT NULL DEFAULT 0,
    callback_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE telegram_update_dedup
    DROP CONSTRAINT IF EXISTS telegram_update_dedup_bot_kind_check;

ALTER TABLE telegram_update_dedup
    ADD CONSTRAINT telegram_update_dedup_bot_kind_check
    CHECK (bot_kind IN ('telegram_client', 'telegram_operator'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_telegram_update_dedup_unique
    ON telegram_update_dedup (channel_account_id, bot_kind, update_id, chat_id, message_id, callback_id);

CREATE TABLE IF NOT EXISTS outbound_messages (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    channel_kind TEXT NOT NULL,
    channel_account_id TEXT NOT NULL REFERENCES channel_accounts(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'queued',
    retry_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    provider_message_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE outbound_messages
    DROP CONSTRAINT IF EXISTS outbound_messages_channel_check;

ALTER TABLE outbound_messages
    ADD CONSTRAINT outbound_messages_channel_check
    CHECK (channel IN ('telegram', 'whatsapp'));

ALTER TABLE outbound_messages
    DROP CONSTRAINT IF EXISTS outbound_messages_channel_kind_check;

ALTER TABLE outbound_messages
    ADD CONSTRAINT outbound_messages_channel_kind_check
    CHECK (channel_kind IN ('telegram_client', 'telegram_operator', 'whatsapp_twilio'));

ALTER TABLE outbound_messages
    DROP CONSTRAINT IF EXISTS outbound_messages_status_check;

ALTER TABLE outbound_messages
    ADD CONSTRAINT outbound_messages_status_check
    CHECK (status IN ('queued', 'processing', 'sent', 'delivered', 'failed'));

CREATE INDEX IF NOT EXISTS idx_outbound_messages_dispatch
    ON outbound_messages (status, next_attempt_at, created_at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL DEFAULT '',
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_workspace_created
    ON audit_logs (workspace_id, created_at DESC);
