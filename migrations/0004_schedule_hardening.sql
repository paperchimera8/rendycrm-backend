INSERT INTO slot_settings (workspace_id, timezone, default_duration_minutes, generation_horizon_days)
SELECT w.id, w.timezone, 60, 30
FROM workspaces w
ON CONFLICT (workspace_id) DO NOTHING;

DELETE FROM slot_holds
WHERE expires_at <= NOW();

UPDATE daily_slots ds
SET slot_date = ((ds.starts_at AT TIME ZONE COALESCE(ss.timezone, w.timezone, 'UTC'))::date)
FROM workspaces w
LEFT JOIN slot_settings ss ON ss.workspace_id = w.id
WHERE w.id = ds.workspace_id
  AND ds.slot_date <> ((ds.starts_at AT TIME ZONE COALESCE(ss.timezone, w.timezone, 'UTC'))::date);

WITH missing_ranges AS (
    SELECT
        b.workspace_id,
        b.starts_at,
        b.ends_at,
        MIN(b.id) AS source_booking_id,
        MIN(b.notes) AS note,
        ((b.starts_at AT TIME ZONE COALESCE(ss.timezone, w.timezone, 'UTC'))::date) AS slot_date,
        (
            SELECT scp.id
            FROM slot_color_presets scp
            WHERE scp.workspace_id = b.workspace_id
            ORDER BY scp.position ASC, scp.created_at ASC, scp.id ASC
            LIMIT 1
        ) AS color_preset_id
    FROM bookings b
    JOIN workspaces w ON w.id = b.workspace_id
    LEFT JOIN slot_settings ss ON ss.workspace_id = b.workspace_id
    WHERE b.status <> 'cancelled'
      AND b.daily_slot_id IS NULL
      AND NOT EXISTS (
          SELECT 1
          FROM daily_slots ds
          WHERE ds.workspace_id = b.workspace_id
            AND ds.starts_at = b.starts_at
            AND ds.ends_at = b.ends_at
      )
    GROUP BY b.workspace_id, b.starts_at, b.ends_at, ((b.starts_at AT TIME ZONE COALESCE(ss.timezone, w.timezone, 'UTC'))::date)
)
INSERT INTO daily_slots (id, workspace_id, slot_date, starts_at, ends_at, duration_minutes, color_preset_id, position, status, is_manual, note)
SELECT
    'dsl_repair_' || SUBSTRING(md5(m.workspace_id || ':' || m.starts_at::text || ':' || m.ends_at::text) FOR 16),
    m.workspace_id,
    m.slot_date,
    m.starts_at,
    m.ends_at,
    GREATEST(1, EXTRACT(EPOCH FROM (m.ends_at - m.starts_at))::integer / 60),
    NULLIF(m.color_preset_id, ''),
    0,
    'free',
    TRUE,
    COALESCE(m.note, '')
FROM missing_ranges m;

UPDATE bookings b
SET daily_slot_id = ds.id
FROM daily_slots ds
WHERE b.workspace_id = ds.workspace_id
  AND b.status <> 'cancelled'
  AND b.starts_at = ds.starts_at
  AND b.ends_at = ds.ends_at
  AND COALESCE(b.daily_slot_id, '') <> ds.id;

UPDATE slot_holds sh
SET daily_slot_id = b.daily_slot_id
FROM bookings b
WHERE b.workspace_id = sh.workspace_id
  AND b.slot_hold_id = sh.id
  AND COALESCE(b.daily_slot_id, '') <> ''
  AND COALESCE(sh.daily_slot_id, '') <> b.daily_slot_id;

WITH ranked AS (
    SELECT
        ds.id,
        ds.workspace_id,
        FIRST_VALUE(ds.id) OVER (
            PARTITION BY ds.workspace_id, ds.starts_at, ds.ends_at
            ORDER BY ds.created_at ASC, ds.id ASC
        ) AS keep_id,
        ROW_NUMBER() OVER (
            PARTITION BY ds.workspace_id, ds.starts_at, ds.ends_at
            ORDER BY ds.created_at ASC, ds.id ASC
        ) AS rn
    FROM daily_slots ds
),
repoint_bookings AS (
    UPDATE bookings b
    SET daily_slot_id = ranked.keep_id
    FROM ranked
    WHERE ranked.rn > 1
      AND b.workspace_id = ranked.workspace_id
      AND b.daily_slot_id = ranked.id
),
repoint_holds AS (
    UPDATE slot_holds sh
    SET daily_slot_id = ranked.keep_id
    FROM ranked
    WHERE ranked.rn > 1
      AND sh.workspace_id = ranked.workspace_id
      AND sh.daily_slot_id = ranked.id
)
DELETE FROM daily_slots ds
USING ranked
WHERE ranked.rn > 1
  AND ds.id = ranked.id
  AND ds.workspace_id = ranked.workspace_id;

WITH hold_rank AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY workspace_id, daily_slot_id
            ORDER BY expires_at DESC, created_at DESC, id DESC
        ) AS rn
    FROM slot_holds
    WHERE daily_slot_id IS NOT NULL
)
DELETE FROM slot_holds sh
USING hold_rank
WHERE sh.id = hold_rank.id
  AND hold_rank.rn > 1;

UPDATE daily_slots ds
SET status = CASE
    WHEN EXISTS (
        SELECT 1
        FROM bookings b
        WHERE b.workspace_id = ds.workspace_id
          AND b.daily_slot_id = ds.id
          AND b.status IN ('confirmed', 'completed')
    ) THEN 'booked'
    WHEN EXISTS (
        SELECT 1
        FROM bookings b
        WHERE b.workspace_id = ds.workspace_id
          AND b.daily_slot_id = ds.id
          AND b.status = 'pending'
    ) OR EXISTS (
        SELECT 1
        FROM slot_holds sh
        WHERE sh.workspace_id = ds.workspace_id
          AND sh.daily_slot_id = ds.id
          AND sh.expires_at > NOW()
    ) THEN 'held'
    WHEN ds.status = 'blocked' THEN 'blocked'
    ELSE 'free'
END
WHERE ds.workspace_id IS NOT NULL;

ALTER TABLE bookings
    ADD CONSTRAINT bookings_time_range_check CHECK (ends_at > starts_at);

ALTER TABLE daily_slots
    ADD CONSTRAINT daily_slots_time_range_check CHECK (ends_at > starts_at);

ALTER TABLE slot_holds
    ADD CONSTRAINT slot_holds_time_range_check CHECK (ends_at > starts_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_daily_slots_workspace_time_unique
    ON daily_slots (workspace_id, starts_at, ends_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bookings_active_daily_slot_unique
    ON bookings (daily_slot_id)
    WHERE daily_slot_id IS NOT NULL AND status <> 'cancelled';

CREATE UNIQUE INDEX IF NOT EXISTS idx_slot_holds_daily_slot_unique
    ON slot_holds (daily_slot_id)
    WHERE daily_slot_id IS NOT NULL;
