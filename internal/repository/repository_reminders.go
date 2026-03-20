package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	. "github.com/vital/rendycrm-app/internal/domain"
)

func setBookingReminderFields(booking *Booking, enabled bool, sentAt sql.NullTime) {
	if booking == nil {
		return
	}
	booking.ClientReminderEnabled = enabled
	if sentAt.Valid {
		value := sentAt.Time
		booking.ClientReminderSentAt = &value
		return
	}
	booking.ClientReminderSentAt = nil
}

func (r *Repository) UpcomingReminderBookings(ctx context.Context, workspaceID string, now time.Time, horizon time.Duration, limit int) ([]Booking, error) {
	if limit <= 0 {
		limit = 10
	}
	start := now.UTC()
	end := start.Add(horizon)
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			b.id,
			b.workspace_id,
			b.customer_id,
			c.name,
			COALESCE(b.daily_slot_id, ''),
			b.starts_at,
			b.ends_at,
			COALESCE(b.amount_cents, 0) / 100,
			b.status,
			b.source,
			b.notes,
			COALESCE(b.client_reminder_enabled, TRUE),
			b.client_reminder_sent_at
		FROM bookings b
		JOIN customers c ON c.id = b.customer_id
		WHERE b.workspace_id = $1
		  AND b.status = 'confirmed'
		  AND b.starts_at >= $2
		  AND b.starts_at < $3
		ORDER BY b.starts_at ASC
		LIMIT $4
	`, workspaceID, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Booking
	for rows.Next() {
		var (
			booking        Booking
			reminderSentAt sql.NullTime
		)
		if err := rows.Scan(
			&booking.ID,
			&booking.WorkspaceID,
			&booking.CustomerID,
			&booking.CustomerName,
			&booking.DailySlotID,
			&booking.StartsAt,
			&booking.EndsAt,
			&booking.Amount,
			&booking.Status,
			&booking.Source,
			&booking.Notes,
			&booking.ClientReminderEnabled,
			&reminderSentAt,
		); err != nil {
			return nil, err
		}
		setBookingReminderFields(&booking, booking.ClientReminderEnabled, reminderSentAt)
		items = append(items, booking)
	}
	return items, rows.Err()
}

func (r *Repository) SetBookingClientReminderEnabled(ctx context.Context, workspaceID, bookingID string, enabled bool) (Booking, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE bookings
		SET client_reminder_enabled = $3
		WHERE id = $1 AND workspace_id = $2
	`, bookingID, workspaceID, enabled)
	if err != nil {
		return Booking{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Booking{}, err
	}
	if affected == 0 {
		return Booking{}, sql.ErrNoRows
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

type dueClientReminder struct {
	BookingID        string
	WorkspaceID      string
	WorkspaceName    string
	CustomerName     string
	ChannelAccountID string
	ChatID           string
	StartsAt         time.Time
	Timezone         string
}

func (r *Repository) EnqueueDueClientTelegramReminders(ctx context.Context, now time.Time, lead time.Duration, limit int) (int, error) {
	if limit <= 0 {
		limit = 20
	}
	now = now.UTC()
	upperBound := now.Add(lead)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT
			b.id,
			b.workspace_id,
			w.name,
			c.name,
			COALESCE(target.channel_account_id, ''),
			COALESCE(identity.external_chat_id, ''),
			b.starts_at,
			COALESCE(NULLIF(ss.timezone, ''), NULLIF(w.timezone, ''), 'UTC')
		FROM bookings b
		JOIN customers c ON c.id = b.customer_id
		JOIN workspaces w ON w.id = b.workspace_id
		LEFT JOIN slot_settings ss ON ss.workspace_id = b.workspace_id
		LEFT JOIN LATERAL (
			SELECT cci.external_id AS external_chat_id
			FROM customer_channel_identities cci
			WHERE cci.workspace_id = b.workspace_id
			  AND cci.customer_id = b.customer_id
			  AND cci.provider = 'telegram'
			ORDER BY cci.created_at DESC, cci.id DESC
			LIMIT 1
		) identity ON TRUE
		LEFT JOIN LATERAL (
			SELECT COALESCE(
				(
					SELECT ca.id
					FROM conversations conv
					JOIN channel_accounts ca ON ca.id = conv.channel_account_id
					WHERE conv.workspace_id = b.workspace_id
					  AND conv.customer_id = b.customer_id
					  AND conv.provider = 'telegram'
					  AND ca.channel_kind = 'telegram_client'
					  AND ca.revoked_at IS NULL
					  AND ca.connected = TRUE
					  AND COALESCE(ca.is_enabled, TRUE) = TRUE
					ORDER BY conv.updated_at DESC, conv.created_at DESC, conv.id DESC
					LIMIT 1
				),
				(
					SELECT ca.id
					FROM client_bot_routes cbr
					JOIN channel_accounts ca ON ca.id = cbr.channel_account_id
					WHERE cbr.selected_workspace_id = b.workspace_id
					  AND cbr.external_chat_id = COALESCE(identity.external_chat_id, '')
					  AND ca.channel_kind = 'telegram_client'
					  AND ca.revoked_at IS NULL
					  AND ca.connected = TRUE
					  AND COALESCE(ca.is_enabled, TRUE) = TRUE
					ORDER BY cbr.updated_at DESC, cbr.created_at DESC, cbr.id DESC
					LIMIT 1
				),
				(
					SELECT ca.id
					FROM channel_accounts ca
					WHERE ca.workspace_id = b.workspace_id
					  AND ca.channel_kind = 'telegram_client'
					  AND COALESCE(ca.account_scope, 'workspace') = 'workspace'
					  AND ca.revoked_at IS NULL
					  AND ca.connected = TRUE
					  AND COALESCE(ca.is_enabled, TRUE) = TRUE
					ORDER BY ca.created_at ASC, ca.id ASC
					LIMIT 1
				),
				(
					SELECT ca.id
					FROM channel_accounts ca
					WHERE ca.channel_kind = 'telegram_client'
					  AND COALESCE(ca.account_scope, 'workspace') = 'global'
					  AND ca.revoked_at IS NULL
					  AND ca.connected = TRUE
					  AND COALESCE(ca.is_enabled, TRUE) = TRUE
					ORDER BY ca.created_at ASC, ca.id ASC
					LIMIT 1
				)
			) AS channel_account_id
		) target ON TRUE
		WHERE b.status = 'confirmed'
		  AND COALESCE(b.client_reminder_enabled, TRUE) = TRUE
		  AND b.client_reminder_sent_at IS NULL
		  AND b.starts_at > $1
		  AND b.starts_at <= $2
		  AND COALESCE(identity.external_chat_id, '') <> ''
		  AND COALESCE(target.channel_account_id, '') <> ''
		ORDER BY b.starts_at ASC
		FOR UPDATE OF b SKIP LOCKED
		LIMIT $3
	`, now, upperBound, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	items := make([]dueClientReminder, 0, limit)
	for rows.Next() {
		var item dueClientReminder
		if err := rows.Scan(
			&item.BookingID,
			&item.WorkspaceID,
			&item.WorkspaceName,
			&item.CustomerName,
			&item.ChannelAccountID,
			&item.ChatID,
			&item.StartsAt,
			&item.Timezone,
		); err != nil {
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	queued := 0
	for _, item := range items {
		payload := TelegramOutboundPayload{
			ChatID: item.ChatID,
			Text:   clientReminderText(item.WorkspaceName, item.StartsAt, item.Timezone),
		}
		if _, err := r.enqueueOutboundMessageTx(ctx, tx, OutboundMessage{
			WorkspaceID:      item.WorkspaceID,
			Channel:          ChannelTelegram,
			ChannelKind:      ChannelKindTelegramClient,
			ChannelAccountID: item.ChannelAccountID,
			Kind:             OutboundKindTelegramSendText,
		}, payload); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE bookings
			SET client_reminder_sent_at = $3
			WHERE id = $1 AND workspace_id = $2
		`, item.BookingID, item.WorkspaceID, now); err != nil {
			return 0, err
		}
		queued++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return queued, nil
}

func clientReminderText(workspaceName string, startsAt time.Time, timezone string) string {
	slotTime := formatReminderStart(startsAt, timezone)
	name := strings.TrimSpace(workspaceName)
	if name == "" {
		return fmt.Sprintf("Напоминание: вы записаны на %s.\nЕсли планы изменились, напишите в этот чат.", slotTime)
	}
	return fmt.Sprintf("Напоминание: вы записаны в %s на %s.\nЕсли планы изменились, напишите в этот чат.", name, slotTime)
}

func formatReminderStart(startsAt time.Time, timezone string) string {
	loc := time.UTC
	if trimmed := strings.TrimSpace(timezone); trimmed != "" {
		if loaded, err := time.LoadLocation(trimmed); err == nil {
			loc = loaded
		}
	}
	return startsAt.In(loc).Format("02.01 в 15:04")
}
