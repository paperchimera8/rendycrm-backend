CREATE TABLE IF NOT EXISTS slot_settings (
    workspace_id TEXT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    default_duration_minutes INTEGER NOT NULL DEFAULT 60,
    generation_horizon_days INTEGER NOT NULL DEFAULT 30,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS slot_color_presets (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    hex TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS slot_templates (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    weekday SMALLINT NOT NULL,
    start_minute INTEGER NOT NULL,
    duration_minutes INTEGER NOT NULL,
    color_preset_id TEXT REFERENCES slot_color_presets(id) ON DELETE SET NULL,
    position INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS daily_slots (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    slot_date DATE NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    duration_minutes INTEGER NOT NULL,
    color_preset_id TEXT REFERENCES slot_color_presets(id) ON DELETE SET NULL,
    position INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'free',
    source_template_id TEXT REFERENCES slot_templates(id) ON DELETE SET NULL,
    is_manual BOOLEAN NOT NULL DEFAULT FALSE,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE slot_holds
    ADD COLUMN IF NOT EXISTS daily_slot_id TEXT REFERENCES daily_slots(id) ON DELETE SET NULL;

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS daily_slot_id TEXT REFERENCES daily_slots(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_slot_color_presets_workspace_position
    ON slot_color_presets (workspace_id, position);

CREATE UNIQUE INDEX IF NOT EXISTS idx_slot_templates_workspace_weekday_position
    ON slot_templates (workspace_id, weekday, position);

CREATE INDEX IF NOT EXISTS idx_daily_slots_workspace_date_position
    ON daily_slots (workspace_id, slot_date, position);

CREATE UNIQUE INDEX IF NOT EXISTS idx_daily_slots_generated_unique
    ON daily_slots (source_template_id, slot_date)
    WHERE source_template_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_daily_slots_workspace_status
    ON daily_slots (workspace_id, status, starts_at);

CREATE INDEX IF NOT EXISTS idx_slot_holds_daily_slot
    ON slot_holds (daily_slot_id);

CREATE INDEX IF NOT EXISTS idx_bookings_daily_slot
    ON bookings (daily_slot_id);
