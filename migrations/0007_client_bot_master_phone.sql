INSERT INTO workspaces (id, name, timezone)
VALUES ('ws_system', 'System Workspace', 'UTC')
ON CONFLICT (id) DO NOTHING;

ALTER TABLE workspaces
    ADD COLUMN IF NOT EXISTS master_phone_raw TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS master_phone_normalized TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_master_phone_unique
    ON workspaces (master_phone_normalized)
    WHERE master_phone_normalized <> '';

ALTER TABLE customer_channel_identities
    ADD COLUMN IF NOT EXISTS workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE;

UPDATE customer_channel_identities cci
SET workspace_id = c.workspace_id
FROM customers c
WHERE c.id = cci.customer_id
  AND cci.workspace_id IS NULL;

ALTER TABLE customer_channel_identities
    ALTER COLUMN workspace_id SET NOT NULL;

ALTER TABLE customer_channel_identities
    DROP CONSTRAINT IF EXISTS customer_channel_identities_provider_external_id_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_channel_identities_workspace_provider_external
    ON customer_channel_identities (workspace_id, provider, external_id);

ALTER TABLE channel_accounts
    ADD COLUMN IF NOT EXISTS account_scope TEXT NOT NULL DEFAULT 'workspace';

ALTER TABLE channel_accounts
    DROP CONSTRAINT IF EXISTS channel_accounts_account_scope_check;

ALTER TABLE channel_accounts
    ADD CONSTRAINT channel_accounts_account_scope_check
    CHECK (account_scope IN ('workspace', 'global'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_accounts_global_kind_unique
    ON channel_accounts (account_scope, channel_kind)
    WHERE account_scope = 'global' AND revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS client_bot_routes (
    id TEXT PRIMARY KEY,
    channel_account_id TEXT NOT NULL REFERENCES channel_accounts(id) ON DELETE CASCADE,
    external_chat_id TEXT NOT NULL,
    selected_workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE,
    selected_master_phone_normalized TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (channel_account_id, external_chat_id)
);

ALTER TABLE client_bot_routes
    DROP CONSTRAINT IF EXISTS client_bot_routes_state_check;

ALTER TABLE client_bot_routes
    ADD CONSTRAINT client_bot_routes_state_check
    CHECK (state IN ('awaiting_master_phone', 'ready'));

CREATE INDEX IF NOT EXISTS idx_client_bot_routes_workspace
    ON client_bot_routes (selected_workspace_id, updated_at DESC);
