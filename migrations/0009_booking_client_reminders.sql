ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS client_reminder_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS client_reminder_sent_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_bookings_client_reminder_due
    ON bookings (starts_at)
    WHERE status = 'confirmed'
      AND client_reminder_enabled = TRUE
      AND client_reminder_sent_at IS NULL;
