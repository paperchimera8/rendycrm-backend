ALTER TABLE conversations
    ADD COLUMN IF NOT EXISTS external_chat_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS intent TEXT NOT NULL DEFAULT 'other',
    ADD COLUMN IF NOT EXISTS last_inbound_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_outbound_at TIMESTAMPTZ;

UPDATE conversations
SET status = CASE
    WHEN status = 'resolved' THEN 'closed'
    WHEN status = 'open' THEN 'human'
    ELSE status
END
WHERE status IN ('open', 'resolved');

UPDATE conversations c
SET external_chat_id = COALESCE(cci.external_id, '')
FROM customer_channel_identities cci
WHERE c.customer_id = cci.customer_id
  AND c.provider = cci.provider
  AND c.external_chat_id = '';

UPDATE conversations c
SET last_inbound_at = inbound.last_created_at
FROM (
    SELECT conversation_id, MAX(created_at) AS last_created_at
    FROM messages
    WHERE direction = 'inbound'
    GROUP BY conversation_id
) inbound
WHERE inbound.conversation_id = c.id
  AND c.last_inbound_at IS NULL;

UPDATE conversations c
SET last_outbound_at = outbound.last_created_at
FROM (
    SELECT conversation_id, MAX(created_at) AS last_created_at
    FROM messages
    WHERE direction = 'outbound'
    GROUP BY conversation_id
) outbound
WHERE outbound.conversation_id = c.id
  AND c.last_outbound_at IS NULL;

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS channel_account_id TEXT REFERENCES channel_accounts(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS provider_meta_json JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE messages m
SET channel_account_id = c.channel_account_id
FROM conversations c
WHERE c.id = m.conversation_id
  AND m.channel_account_id IS NULL;

CREATE TABLE IF NOT EXISTS operator_bot_bindings (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    telegram_user_id TEXT NOT NULL,
    telegram_chat_id TEXT NOT NULL,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (telegram_chat_id),
    UNIQUE (telegram_user_id)
);

CREATE TABLE IF NOT EXISTS operator_bot_link_codes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bot_sessions (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    state TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, scope, actor_type, actor_id)
);

ALTER TABLE conversations
    DROP CONSTRAINT IF EXISTS conversations_status_check;

ALTER TABLE conversations
    ADD CONSTRAINT conversations_status_check
    CHECK (status IN ('new', 'auto', 'human', 'booked', 'closed'));

ALTER TABLE conversations
    DROP CONSTRAINT IF EXISTS conversations_provider_check;

ALTER TABLE conversations
    ADD CONSTRAINT conversations_provider_check
    CHECK (provider IN ('telegram', 'whatsapp'));

ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_direction_check;

ALTER TABLE messages
    ADD CONSTRAINT messages_direction_check
    CHECK (direction IN ('inbound', 'outbound'));

ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_sender_type_check;

ALTER TABLE messages
    ADD CONSTRAINT messages_sender_type_check
    CHECK (sender_type IN ('customer', 'operator', 'bot'));

ALTER TABLE messages
    DROP CONSTRAINT IF EXISTS messages_delivery_status_check;

ALTER TABLE messages
    ADD CONSTRAINT messages_delivery_status_check
    CHECK (delivery_status IN ('queued', 'sent', 'delivered', 'failed', 'received'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversations_workspace_provider_external_open
    ON conversations (workspace_id, provider, external_chat_id)
    WHERE status <> 'closed' AND external_chat_id <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_channel_account_external_message_unique
    ON messages (channel_account_id, external_message_id)
    WHERE channel_account_id IS NOT NULL AND external_message_id <> '';

CREATE INDEX IF NOT EXISTS idx_conversations_workspace_status_updated
    ON conversations (workspace_id, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversations_workspace_intent
    ON conversations (workspace_id, intent, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_operator_bot_bindings_workspace
    ON operator_bot_bindings (workspace_id, linked_at DESC);

CREATE INDEX IF NOT EXISTS idx_operator_bot_link_codes_user
    ON operator_bot_link_codes (user_id, expires_at DESC);
