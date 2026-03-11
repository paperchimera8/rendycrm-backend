package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/vital/rendycrm-app/internal/domain"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Login(ctx context.Context, email, password string) (User, Workspace, error) {
	const query = `
		SELECT u.id, u.email, u.name, wm.role, w.id, w.name, u.password_hash
		FROM users u
		JOIN workspace_members wm ON wm.user_id = u.id
		JOIN workspaces w ON w.id = wm.workspace_id
		WHERE LOWER(u.email) = LOWER($1)
		LIMIT 1
	`
	var user User
	var workspace Workspace
	var passwordHash string
	if err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.Role,
		&workspace.ID,
		&workspace.Name,
		&passwordHash,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, Workspace{}, errors.New("invalid credentials")
		}
		return User{}, Workspace{}, err
	}
	if passwordHash != hashToken(password) {
		return User{}, Workspace{}, errors.New("invalid credentials")
	}
	return user, workspace, nil
}

func (r *Repository) SaveSessionMetadata(ctx context.Context, session Session) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, workspace_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (token_hash) DO UPDATE
		SET expires_at = EXCLUDED.expires_at
	`, "db_"+session.Token, session.UserID, session.WorkspaceID, hashToken(session.Token), session.ExpiresAt)
	return err
}

func (r *Repository) DeleteSessionMetadata(ctx context.Context, token string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

func (r *Repository) Me(ctx context.Context, userID, workspaceID string) (User, Workspace, error) {
	const query = `
		SELECT u.id, u.email, u.name, wm.role, w.id, w.name
		FROM users u
		JOIN workspace_members wm ON wm.user_id = u.id AND wm.workspace_id = $2
		JOIN workspaces w ON w.id = wm.workspace_id
		WHERE u.id = $1
	`
	var user User
	var workspace Workspace
	if err := r.db.QueryRowContext(ctx, query, userID, workspaceID).Scan(
		&user.ID, &user.Email, &user.Name, &user.Role, &workspace.ID, &workspace.Name,
	); err != nil {
		return User{}, Workspace{}, err
	}
	return user, workspace, nil
}

func (r *Repository) Dashboard(ctx context.Context, workspaceID string) (Dashboard, error) {
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := todayStart.Add(24 * time.Hour)
	var dashboard Dashboard
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE starts_at >= $2 AND starts_at < $3) AS today_bookings,
			COUNT(*) FILTER (WHERE status = 'pending') AS awaiting_confirmation,
			COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled_bookings
		FROM bookings
		WHERE workspace_id = $1
	`, workspaceID, todayStart, todayEnd).Scan(
		&dashboard.TodayBookings,
		&dashboard.AwaitingConfirmation,
		&dashboard.CancelledBookings,
	); err != nil {
		return Dashboard{}, err
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(unread_count), 0),
			COUNT(*) FILTER (WHERE status <> 'closed')
		FROM conversations
		WHERE workspace_id = $1
	`, workspaceID).Scan(&dashboard.NewMessages, &dashboard.ActiveConversations); err != nil {
		return Dashboard{}, err
	}
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM reviews
		WHERE workspace_id = $1 AND status = 'open'
	`, workspaceID).Scan(&dashboard.NewReviews); err != nil {
		return Dashboard{}, err
	}
	return dashboard, nil
}

func (r *Repository) Conversations(ctx context.Context, workspaceID string) ([]Conversation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.id,
			c.workspace_id,
			c.customer_id,
			c.provider,
			COALESCE(c.external_chat_id, ''),
			cust.name,
			c.status,
			COALESCE(c.assigned_user_id, ''),
			c.unread_count,
			c.ai_summary,
			COALESCE(c.intent, 'other'),
			c.last_message_text,
			COALESCE(c.last_inbound_at, 'epoch'::timestamptz),
			COALESCE(c.last_outbound_at, 'epoch'::timestamptz),
			c.updated_at
		FROM conversations c
		JOIN customers cust ON cust.id = c.customer_id
		WHERE c.workspace_id = $1
		ORDER BY c.updated_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Conversation
	for rows.Next() {
		var item Conversation
		if err := rows.Scan(
			&item.ID,
			&item.WorkspaceID,
			&item.CustomerID,
			&item.Provider,
			&item.ExternalChatID,
			&item.Title,
			&item.Status,
			&item.AssignedUserID,
			&item.UnreadCount,
			&item.AISummary,
			&item.Intent,
			&item.LastMessageText,
			&item.LastInboundAt,
			&item.LastOutboundAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Channel = item.Provider
		item.AssignedOperatorID = item.AssignedUserID
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ConversationDetail(ctx context.Context, workspaceID, conversationID string) (Conversation, []Message, Customer, error) {
	var conversation Conversation
	var customer Customer
	const conversationQuery = `
		SELECT
			c.id,
			c.workspace_id,
			c.customer_id,
			c.provider,
			COALESCE(c.external_chat_id, ''),
			cust.name,
			c.status,
			COALESCE(c.assigned_user_id, ''),
			c.unread_count,
			c.ai_summary,
			COALESCE(c.intent, 'other'),
			c.last_message_text,
			COALESCE(c.last_inbound_at, 'epoch'::timestamptz),
			COALESCE(c.last_outbound_at, 'epoch'::timestamptz),
			c.updated_at
		FROM conversations c
		JOIN customers cust ON cust.id = c.customer_id
		WHERE c.workspace_id = $1 AND c.id = $2
	`
	if err := r.db.QueryRowContext(ctx, conversationQuery, workspaceID, conversationID).Scan(
		&conversation.ID,
		&conversation.WorkspaceID,
		&conversation.CustomerID,
		&conversation.Provider,
		&conversation.ExternalChatID,
		&conversation.Title,
		&conversation.Status,
		&conversation.AssignedUserID,
		&conversation.UnreadCount,
		&conversation.AISummary,
		&conversation.Intent,
		&conversation.LastMessageText,
		&conversation.LastInboundAt,
		&conversation.LastOutboundAt,
		&conversation.UpdatedAt,
	); err != nil {
		return Conversation{}, nil, Customer{}, err
	}
	conversation.Channel = conversation.Provider
	conversation.AssignedOperatorID = conversation.AssignedUserID
	customer, err := r.Customer(ctx, workspaceID, conversation.CustomerID)
	if err != nil {
		return Conversation{}, nil, Customer{}, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, conversation_id, direction, sender_type, body, delivery_status, COALESCE(external_message_id, ''), created_at
		FROM messages
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`, conversationID)
	if err != nil {
		return Conversation{}, nil, Customer{}, err
	}
	defer rows.Close()
	messages := []Message{}
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.ConversationID, &message.Direction, &message.SenderType, &message.Text, &message.Status, &message.ExternalID, &message.CreatedAt); err != nil {
			return Conversation{}, nil, Customer{}, err
		}
		message.DeliveryStatus = message.Status
		messages = append(messages, message)
	}
	return conversation, messages, customer, rows.Err()
}

func (r *Repository) Reply(ctx context.Context, workspaceID, conversationID, userID, text string) (Conversation, Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Conversation{}, Message{}, err
	}
	defer tx.Rollback()
	var customerName string
	targetConversation, account, err := r.outboundTargetTx(ctx, tx, workspaceID, conversationID)
	if err != nil {
		return Conversation{}, Message{}, err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT cust.name
		FROM conversations c
		JOIN customers cust ON cust.id = c.customer_id
		WHERE c.id = $1 AND c.workspace_id = $2
	`, conversationID, workspaceID).Scan(&customerName); err != nil {
		return Conversation{}, Message{}, err
	}
	message := Message{
		ID:             newID("msg"),
		ConversationID: conversationID,
		Direction:      MessageOutbound,
		SenderType:     MessageSenderOperator,
		Text:           text,
		Status:         string(MessageDeliveryQueued),
		DeliveryStatus: string(MessageDeliveryQueued),
		CreatedAt:      time.Now().UTC(),
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, workspace_id, channel_account_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at)
		VALUES ($1, $2, $3, $4, '', $5, $6, $7, $8, $9, $10)
	`, message.ID, conversationID, workspaceID, account.ID, message.Direction, message.SenderType, message.Text, message.DeliveryStatus, message.ID, message.CreatedAt); err != nil {
		return Conversation{}, Message{}, err
	}
	payload := TelegramOutboundPayload{
		ChatID: targetConversation.ExternalChatID,
		Text:   text,
	}
	if account.Provider == ChannelTelegram {
		if _, err := r.enqueueOutboundMessageTx(ctx, tx, OutboundMessage{
			WorkspaceID:      workspaceID,
			Channel:          account.Provider,
			ChannelKind:      account.ChannelKind,
			ChannelAccountID: account.ID,
			ConversationID:   conversationID,
			MessageID:        message.ID,
			Kind:             OutboundKindTelegramSendText,
		}, payload); err != nil {
			return Conversation{}, Message{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET assigned_user_id = $3, last_message_text = $4, unread_count = 0, updated_at = $5, last_outbound_at = $5, status = 'human'
		WHERE id = $1 AND workspace_id = $2
	`, conversationID, workspaceID, userID, text, message.CreatedAt); err != nil {
		return Conversation{}, Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Conversation{}, Message{}, err
	}
	return Conversation{
		ID:                 conversationID,
		WorkspaceID:        workspaceID,
		Provider:           account.Provider,
		Channel:            account.Provider,
		Title:              customerName,
		Status:             ConversationHuman,
		AssignedUserID:     userID,
		AssignedOperatorID: userID,
		UnreadCount:        0,
		LastMessageText:    text,
		LastOutboundAt:     message.CreatedAt,
		UpdatedAt:          message.CreatedAt,
	}, message, nil
}

func (r *Repository) AssignConversation(ctx context.Context, workspaceID, conversationID, userID string) (Conversation, error) {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE conversations
		SET assigned_user_id = $3, updated_at = NOW()
		WHERE id = $1 AND workspace_id = $2
	`, conversationID, workspaceID, userID); err != nil {
		return Conversation{}, err
	}
	conversation, _, _, err := r.ConversationDetail(ctx, workspaceID, conversationID)
	return conversation, err
}

func (r *Repository) UpdateConversationStatus(ctx context.Context, workspaceID, conversationID string, status ConversationStatus) (Conversation, error) {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE conversations
		SET status = $3, updated_at = NOW()
		WHERE id = $1 AND workspace_id = $2
	`, conversationID, workspaceID, status); err != nil {
		return Conversation{}, err
	}
	conversation, _, _, err := r.ConversationDetail(ctx, workspaceID, conversationID)
	return conversation, err
}

func (r *Repository) Customer(ctx context.Context, workspaceID, customerID string) (Customer, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Customer{}, err
	}
	defer tx.Rollback()
	customer, err := r.customerTx(ctx, tx, workspaceID, customerID)
	if err != nil {
		return Customer{}, err
	}
	if err := tx.Commit(); err != nil {
		return Customer{}, err
	}
	return customer, nil
}

func (r *Repository) UpdateCustomerName(ctx context.Context, workspaceID, customerID, name string) (Customer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Customer{}, errors.New("customer name is required")
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE customers
		SET name = $3
		WHERE id = $1 AND workspace_id = $2
	`, customerID, workspaceID, name); err != nil {
		return Customer{}, err
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE conversations
		SET updated_at = updated_at
		WHERE customer_id = $1 AND workspace_id = $2
	`, customerID, workspaceID); err != nil {
		return Customer{}, err
	}
	return r.Customer(ctx, workspaceID, customerID)
}

func (r *Repository) Availability(ctx context.Context, workspaceID string) ([]AvailabilityRule, []AvailabilityException, []Booking, []SlotHold, error) {
	ruleRows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, day_of_week, start_minute, end_minute, enabled
		FROM availability_rules
		WHERE workspace_id = $1
		ORDER BY day_of_week, start_minute
	`, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer ruleRows.Close()
	var rules []AvailabilityRule
	for ruleRows.Next() {
		var rule AvailabilityRule
		if err := ruleRows.Scan(&rule.ID, &rule.WorkspaceID, &rule.DayOfWeek, &rule.StartMinute, &rule.EndMinute, &rule.Enabled); err != nil {
			return nil, nil, nil, nil, err
		}
		rules = append(rules, rule)
	}
	if err := ruleRows.Err(); err != nil {
		return nil, nil, nil, nil, err
	}
	exRows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, starts_at, ends_at, reason
		FROM availability_exceptions
		WHERE workspace_id = $1
		ORDER BY starts_at ASC
	`, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer exRows.Close()
	var exceptions []AvailabilityException
	for exRows.Next() {
		var exception AvailabilityException
		if err := exRows.Scan(&exception.ID, &exception.WorkspaceID, &exception.StartsAt, &exception.EndsAt, &exception.Reason); err != nil {
			return nil, nil, nil, nil, err
		}
		exceptions = append(exceptions, exception)
	}
	if err := exRows.Err(); err != nil {
		return nil, nil, nil, nil, err
	}
	bookingRows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, customer_id, COALESCE(daily_slot_id, ''), starts_at, ends_at, status, source, notes
		FROM bookings
		WHERE workspace_id = $1
	`, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer bookingRows.Close()
	var bookings []Booking
	for bookingRows.Next() {
		var booking Booking
		if err := bookingRows.Scan(&booking.ID, &booking.WorkspaceID, &booking.CustomerID, &booking.DailySlotID, &booking.StartsAt, &booking.EndsAt, &booking.Status, &booking.Source, &booking.Notes); err != nil {
			return nil, nil, nil, nil, err
		}
		bookings = append(bookings, booking)
	}
	if err := bookingRows.Err(); err != nil {
		return nil, nil, nil, nil, err
	}
	holdRows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, customer_id, COALESCE(daily_slot_id, ''), starts_at, ends_at, expires_at
		FROM slot_holds
		WHERE workspace_id = $1 AND expires_at > NOW()
	`, workspaceID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer holdRows.Close()
	var holds []SlotHold
	for holdRows.Next() {
		var hold SlotHold
		if err := holdRows.Scan(&hold.ID, &hold.WorkspaceID, &hold.CustomerID, &hold.DailySlotID, &hold.StartsAt, &hold.EndsAt, &hold.ExpiresAt); err != nil {
			return nil, nil, nil, nil, err
		}
		holds = append(holds, hold)
	}
	return rules, exceptions, bookings, holds, holdRows.Err()
}

func (r *Repository) ReplaceAvailabilityRules(ctx context.Context, workspaceID string, rules []AvailabilityRule) ([]AvailabilityRule, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM availability_rules WHERE workspace_id = $1`, workspaceID); err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == "" {
			rules[i].ID = newID("avr")
		}
		rules[i].WorkspaceID = workspaceID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO availability_rules (id, workspace_id, day_of_week, start_minute, end_minute, enabled)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, rules[i].ID, workspaceID, rules[i].DayOfWeek, rules[i].StartMinute, rules[i].EndMinute, rules[i].Enabled); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *Repository) ReplaceAvailabilityExceptions(ctx context.Context, workspaceID string, exceptions []AvailabilityException) ([]AvailabilityException, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM availability_exceptions WHERE workspace_id = $1`, workspaceID); err != nil {
		return nil, err
	}
	for i := range exceptions {
		if exceptions[i].ID == "" {
			exceptions[i].ID = newID("avx")
		}
		exceptions[i].WorkspaceID = workspaceID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO availability_exceptions (id, workspace_id, starts_at, ends_at, reason)
			VALUES ($1, $2, $3, $4, $5)
		`, exceptions[i].ID, workspaceID, exceptions[i].StartsAt, exceptions[i].EndsAt, exceptions[i].Reason); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return exceptions, nil
}

func (r *Repository) bookingConflictExistsTx(ctx context.Context, tx *sql.Tx, workspaceID string, startsAt, endsAt time.Time, excludeBookingID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM bookings
			WHERE workspace_id = $1
				AND status <> 'cancelled'
				AND starts_at < $3
				AND ends_at > $2
	`
	args := []any{workspaceID, startsAt, endsAt}
	if excludeBookingID != "" {
		query += ` AND id <> $4`
		args = append(args, excludeBookingID)
	}
	query += `
		) OR EXISTS (
			SELECT 1 FROM slot_holds
			WHERE workspace_id = $1
				AND expires_at > NOW()
				AND starts_at < $3
				AND ends_at > $2
	`
	if excludeBookingID != "" {
		query += ` AND NOT EXISTS (SELECT 1 FROM bookings b WHERE b.id = $4 AND b.workspace_id = $1 AND b.slot_hold_id = slot_holds.id)`
	}
	query += `
		)
	`
	var exists bool
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) defaultSlotColorIDTx(ctx context.Context, tx *sql.Tx, workspaceID string) (string, error) {
	var colorID string
	if err := tx.QueryRowContext(ctx, `
		SELECT id
		FROM slot_color_presets
		WHERE workspace_id = $1
		ORDER BY position ASC, created_at ASC
		LIMIT 1
	`, workspaceID).Scan(&colorID); err != nil {
		return "", err
	}
	return colorID, nil
}

func (r *Repository) exactSlotTx(ctx context.Context, tx *sql.Tx, workspaceID string, startsAt, endsAt time.Time) (string, DailySlotStatus, error) {
	var (
		slotID string
		status DailySlotStatus
	)
	err := tx.QueryRowContext(ctx, `
		SELECT id, status
		FROM daily_slots
		WHERE workspace_id = $1 AND starts_at = $2 AND ends_at = $3
		ORDER BY id ASC
		LIMIT 1
		FOR UPDATE
	`, workspaceID, startsAt, endsAt).Scan(&slotID, &status)
	if err != nil {
		return "", "", err
	}
	return slotID, status, nil
}

func (r *Repository) overlappingDailySlotsExistTx(ctx context.Context, tx *sql.Tx, workspaceID, excludeSlotID string, startsAt, endsAt time.Time) (bool, error) {
	var exists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM daily_slots
			WHERE workspace_id = $1
				AND id <> $2
				AND starts_at < $4
				AND ends_at > $3
		)
	`, workspaceID, excludeSlotID, startsAt, endsAt).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) createManualDailySlotTx(ctx context.Context, tx *sql.Tx, workspaceID, colorPresetID string, startsAt, endsAt time.Time) (string, error) {
	slotDate, err := r.slotDateForTimeTx(ctx, tx, workspaceID, startsAt)
	if err != nil {
		return "", err
	}
	slotID := newID("dsl")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO daily_slots (id, workspace_id, slot_date, starts_at, ends_at, duration_minutes, color_preset_id, position, status, is_manual, note)
		VALUES (
			$1, $2, $3, $4, $5,
			$6,
			NULLIF($7, ''),
			COALESCE((SELECT MAX(position) + 1 FROM daily_slots WHERE workspace_id = $2 AND slot_date = $3), 0),
			'free',
			TRUE,
			''
		)
	`, slotID, workspaceID, slotDate, startsAt, endsAt, int(endsAt.Sub(startsAt).Minutes()), colorPresetID); err != nil {
		return "", err
	}
	return slotID, nil
}

func (r *Repository) ensureBookedSlotForRangeTx(ctx context.Context, tx *sql.Tx, workspaceID, excludeBookingID, excludeSlotID string, startsAt, endsAt time.Time) (string, error) {
	if !endsAt.After(startsAt) {
		return "", errors.New("invalid time range")
	}
	slotID, status, err := r.exactSlotTx(ctx, tx, workspaceID, startsAt, endsAt)
	if err == nil {
		if excludeSlotID != "" && slotID == excludeSlotID {
			return slotID, nil
		}
		if status == DailySlotBlocked {
			return "", errors.New("slot unavailable")
		}
		conflict, err := r.bookingConflictExistsTx(ctx, tx, workspaceID, startsAt, endsAt, excludeBookingID)
		if err != nil {
			return "", err
		}
		if conflict {
			return "", errors.New("slot unavailable")
		}
		return slotID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	conflict, err := r.bookingConflictExistsTx(ctx, tx, workspaceID, startsAt, endsAt, excludeBookingID)
	if err != nil {
		return "", err
	}
	if conflict {
		return "", errors.New("slot unavailable")
	}
	overlap, err := r.overlappingDailySlotsExistTx(ctx, tx, workspaceID, excludeSlotID, startsAt, endsAt)
	if err != nil {
		return "", err
	}
	if overlap {
		return "", errors.New("slot unavailable")
	}
	colorID, err := r.defaultSlotColorIDTx(ctx, tx, workspaceID)
	if err != nil {
		return "", err
	}
	return r.createManualDailySlotTx(ctx, tx, workspaceID, colorID, startsAt, endsAt)
}

func (r *Repository) freeSlotIfUnusedTx(ctx context.Context, tx *sql.Tx, workspaceID, slotID, excludeBookingID string) error {
	if slotID == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE daily_slots
		SET status = 'free'
		WHERE id = $1
			AND workspace_id = $2
			AND NOT EXISTS (
				SELECT 1 FROM bookings
				WHERE daily_slot_id = $1
					AND workspace_id = $2
					AND status <> 'cancelled'
					AND id <> $3
			)
	`, slotID, workspaceID, excludeBookingID)
	return err
}

func (r *Repository) CreateBookingForDailySlot(ctx context.Context, workspaceID, customerID, dailySlotID, notes string) (Booking, SlotHold, error) {
	if err := r.cleanupExpiredSlotHolds(ctx, workspaceID); err != nil {
		return Booking{}, SlotHold{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, SlotHold{}, err
	}
	defer tx.Rollback()

	var (
		startsAt time.Time
		endsAt   time.Time
		status   DailySlotStatus
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT starts_at, ends_at, status
		FROM daily_slots
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, dailySlotID, workspaceID).Scan(&startsAt, &endsAt, &status); err != nil {
		return Booking{}, SlotHold{}, err
	}
	if status != DailySlotFree {
		return Booking{}, SlotHold{}, errors.New("slot unavailable")
	}

	hold := SlotHold{
		ID:          newID("hold"),
		WorkspaceID: workspaceID,
		CustomerID:  customerID,
		DailySlotID: dailySlotID,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		ExpiresAt:   time.Now().UTC().Add(15 * time.Minute),
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO slot_holds (id, workspace_id, customer_id, daily_slot_id, starts_at, ends_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, hold.ID, workspaceID, customerID, dailySlotID, startsAt, endsAt, hold.ExpiresAt); err != nil {
		return Booking{}, SlotHold{}, err
	}

	booking := Booking{
		ID:          newID("bok"),
		WorkspaceID: workspaceID,
		CustomerID:  customerID,
		DailySlotID: dailySlotID,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		Status:      BookingPending,
		Source:      "operator",
		Notes:       notes,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bookings (id, workspace_id, customer_id, daily_slot_id, slot_hold_id, starts_at, ends_at, amount_cents, status, source, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $8, $9, $10)
	`, booking.ID, workspaceID, customerID, dailySlotID, hold.ID, startsAt, endsAt, booking.Status, booking.Source, notes); err != nil {
		return Booking{}, SlotHold{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, dailySlotID); err != nil {
		return Booking{}, SlotHold{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, SlotHold{}, err
	}
	stored, err := r.Booking(ctx, workspaceID, booking.ID)
	if err != nil {
		return Booking{}, SlotHold{}, err
	}
	return stored, hold, nil
}

func (r *Repository) CreateConfirmedBookingForDailySlot(ctx context.Context, workspaceID, customerID, dailySlotID string, amount int, notes string) (Booking, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	var (
		startsAt time.Time
		endsAt   time.Time
		status   DailySlotStatus
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT starts_at, ends_at, status
		FROM daily_slots
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, dailySlotID, workspaceID).Scan(&startsAt, &endsAt, &status); err != nil {
		return Booking{}, err
	}
	if status != DailySlotFree {
		return Booking{}, errors.New("slot unavailable")
	}
	bookingID := newID("bok")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bookings (id, workspace_id, customer_id, daily_slot_id, starts_at, ends_at, amount_cents, status, source, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'confirmed', 'operator', $8)
	`, bookingID, workspaceID, customerID, dailySlotID, startsAt, endsAt, amount*100, notes); err != nil {
		return Booking{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, dailySlotID); err != nil {
		return Booking{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, dailySlotID); err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

func (r *Repository) CreateConfirmedBooking(ctx context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, amount int, notes string) (Booking, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	slotID, err := r.ensureBookedSlotForRangeTx(ctx, tx, workspaceID, "", "", startsAt, endsAt)
	if err != nil {
		return Booking{}, err
	}
	bookingID := newID("bok")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bookings (id, workspace_id, customer_id, daily_slot_id, starts_at, ends_at, amount_cents, status, source, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'confirmed', 'operator', $8)
	`, bookingID, workspaceID, customerID, slotID, startsAt, endsAt, amount*100, notes); err != nil {
		return Booking{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, slotID); err != nil {
		return Booking{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, slotID); err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

func (r *Repository) CreateBooking(ctx context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, notes string) (Booking, SlotHold, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, SlotHold{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, SlotHold{}, err
	}
	defer tx.Rollback()

	slotID, err := r.ensureBookedSlotForRangeTx(ctx, tx, workspaceID, "", "", startsAt, endsAt)
	if err != nil {
		return Booking{}, SlotHold{}, err
	}
	hold := SlotHold{
		ID:          newID("hold"),
		WorkspaceID: workspaceID,
		CustomerID:  customerID,
		DailySlotID: slotID,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		ExpiresAt:   time.Now().UTC().Add(15 * time.Minute),
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO slot_holds (id, workspace_id, customer_id, daily_slot_id, starts_at, ends_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, hold.ID, workspaceID, customerID, slotID, startsAt, endsAt, hold.ExpiresAt); err != nil {
		return Booking{}, SlotHold{}, err
	}
	booking := Booking{
		ID:           newID("bok"),
		WorkspaceID:  workspaceID,
		CustomerID:   customerID,
		CustomerName: "",
		DailySlotID:  slotID,
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		Status:       BookingPending,
		Source:       "operator",
		Notes:        notes,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bookings (id, workspace_id, customer_id, daily_slot_id, slot_hold_id, starts_at, ends_at, amount_cents, status, source, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $8, $9, $10)
	`, booking.ID, workspaceID, customerID, slotID, hold.ID, startsAt, endsAt, booking.Status, booking.Source, notes); err != nil {
		return Booking{}, SlotHold{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, slotID); err != nil {
		return Booking{}, SlotHold{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, SlotHold{}, err
	}
	return booking, hold, nil
}

func (r *Repository) UpdateBookingStatus(ctx context.Context, workspaceID, bookingID string, status BookingStatus, amount *int) (Booking, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	var (
		dailySlotID string
		slotHoldID  string
		startsAt    time.Time
		endsAt      time.Time
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(daily_slot_id, ''), COALESCE(slot_hold_id, ''), starts_at, ends_at
		FROM bookings
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, bookingID, workspaceID).Scan(&dailySlotID, &slotHoldID, &startsAt, &endsAt); err != nil {
		return Booking{}, err
	}

	if (status == BookingConfirmed || status == BookingCompleted) && dailySlotID == "" {
		dailySlotID, err = r.ensureBookedSlotForRangeTx(ctx, tx, workspaceID, bookingID, "", startsAt, endsAt)
		if err != nil {
			return Booking{}, err
		}
	}

	if amount != nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE bookings
			SET daily_slot_id = NULLIF($3, ''),
				slot_hold_id = CASE WHEN $4 IN ('confirmed', 'completed', 'cancelled') THEN NULL ELSE slot_hold_id END,
				status = $4,
				amount_cents = $5
			WHERE id = $1 AND workspace_id = $2
		`, bookingID, workspaceID, dailySlotID, status, *amount*100); err != nil {
			return Booking{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE bookings
			SET daily_slot_id = NULLIF($3, ''),
				slot_hold_id = CASE WHEN $4 IN ('confirmed', 'completed', 'cancelled') THEN NULL ELSE slot_hold_id END,
				status = $4
			WHERE id = $1 AND workspace_id = $2
		`, bookingID, workspaceID, dailySlotID, status); err != nil {
			return Booking{}, err
		}
	}

	if slotHoldID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE id = $1 AND workspace_id = $2`, slotHoldID, workspaceID); err != nil {
			return Booking{}, err
		}
	}
	if dailySlotID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, dailySlotID); err != nil {
			return Booking{}, err
		}
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, dailySlotID); err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

func (r *Repository) RescheduleBooking(ctx context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, notes string) (Booking, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	var (
		customerID string
		oldSlotID  string
		oldHoldID  string
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT customer_id, COALESCE(daily_slot_id, ''), COALESCE(slot_hold_id, '')
		FROM bookings
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, bookingID, workspaceID).Scan(&customerID, &oldSlotID, &oldHoldID); err != nil {
		return Booking{}, err
	}

	slotID, err := r.ensureBookedSlotForRangeTx(ctx, tx, workspaceID, bookingID, oldSlotID, startsAt, endsAt)
	if err != nil {
		return Booking{}, err
	}

	hold := SlotHold{
		ID:          newID("hold"),
		WorkspaceID: workspaceID,
		CustomerID:  customerID,
		DailySlotID: slotID,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		ExpiresAt:   time.Now().UTC().Add(15 * time.Minute),
	}
	if oldHoldID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE id = $1 AND workspace_id = $2`, oldHoldID, workspaceID); err != nil {
			return Booking{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO slot_holds (id, workspace_id, customer_id, daily_slot_id, starts_at, ends_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, hold.ID, workspaceID, customerID, slotID, startsAt, endsAt, hold.ExpiresAt); err != nil {
		return Booking{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE bookings
		SET daily_slot_id = $3,
			slot_hold_id = $4,
			starts_at = $5,
			ends_at = $6,
			status = $7,
			notes = $8
		WHERE id = $1 AND workspace_id = $2
	`, bookingID, workspaceID, slotID, hold.ID, startsAt, endsAt, BookingPending, notes); err != nil {
		return Booking{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2 AND id <> $3`, workspaceID, slotID, hold.ID); err != nil {
		return Booking{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, oldSlotID, slotID); err != nil {
		return Booking{}, err
	}

	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

func (r *Repository) RescheduleConfirmedBooking(ctx context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, amount int, notes string) (Booking, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	var (
		oldSlotID string
		oldHoldID string
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(daily_slot_id, ''), COALESCE(slot_hold_id, '')
		FROM bookings
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, bookingID, workspaceID).Scan(&oldSlotID, &oldHoldID); err != nil {
		return Booking{}, err
	}

	slotID, err := r.ensureBookedSlotForRangeTx(ctx, tx, workspaceID, bookingID, oldSlotID, startsAt, endsAt)
	if err != nil {
		return Booking{}, err
	}

	if oldHoldID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE id = $1 AND workspace_id = $2`, oldHoldID, workspaceID); err != nil {
			return Booking{}, err
		}
	}
	if oldSlotID != "" && oldSlotID != slotID {
		if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, oldSlotID); err != nil {
			return Booking{}, err
		}
		if err := r.freeSlotIfUnusedTx(ctx, tx, workspaceID, oldSlotID, bookingID); err != nil {
			return Booking{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE bookings
		SET daily_slot_id = $3,
			slot_hold_id = NULL,
			starts_at = $4,
			ends_at = $5,
			amount_cents = $6,
			status = 'confirmed',
			notes = $7
		WHERE id = $1 AND workspace_id = $2
	`, bookingID, workspaceID, slotID, startsAt, endsAt, amount*100, notes); err != nil {
		return Booking{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, slotID); err != nil {
		return Booking{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, oldSlotID, slotID); err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

func (r *Repository) RescheduleBookingToDailySlot(ctx context.Context, workspaceID, bookingID, dailySlotID, notes string) (Booking, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	if err := r.cleanupExpiredSlotHolds(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	var (
		oldSlotID  string
		oldHoldID  string
		customerID string
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT customer_id, COALESCE(daily_slot_id, ''), COALESCE(slot_hold_id, '')
		FROM bookings
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, bookingID, workspaceID).Scan(&customerID, &oldSlotID, &oldHoldID); err != nil {
		return Booking{}, err
	}

	var (
		startsAt time.Time
		endsAt   time.Time
		status   DailySlotStatus
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT starts_at, ends_at, status
		FROM daily_slots
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, dailySlotID, workspaceID).Scan(&startsAt, &endsAt, &status); err != nil {
		return Booking{}, err
	}
	if status != DailySlotFree && dailySlotID != oldSlotID {
		return Booking{}, errors.New("slot unavailable")
	}

	holdID := oldHoldID
	if dailySlotID != oldSlotID || holdID == "" {
		if oldHoldID != "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE id = $1 AND workspace_id = $2`, oldHoldID, workspaceID); err != nil {
				return Booking{}, err
			}
		}
		holdID = newID("hold")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO slot_holds (id, workspace_id, customer_id, daily_slot_id, starts_at, ends_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, holdID, workspaceID, customerID, dailySlotID, startsAt, endsAt, time.Now().UTC().Add(15*time.Minute)); err != nil {
			return Booking{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE bookings
		SET daily_slot_id = $3,
			slot_hold_id = NULLIF($4, ''),
			starts_at = $5,
			ends_at = $6,
			status = $7,
			notes = $8
		WHERE id = $1 AND workspace_id = $2
	`, bookingID, workspaceID, dailySlotID, holdID, startsAt, endsAt, BookingPending, notes); err != nil {
		return Booking{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2 AND id <> NULLIF($3, '')`, workspaceID, dailySlotID, holdID); err != nil {
		return Booking{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, oldSlotID, dailySlotID); err != nil {
		return Booking{}, err
	}
	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

func (r *Repository) RescheduleConfirmedBookingToDailySlot(ctx context.Context, workspaceID, bookingID, dailySlotID string, amount int, notes string) (Booking, error) {
	if err := r.EnsureSlotSystem(ctx, workspaceID); err != nil {
		return Booking{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Booking{}, err
	}
	defer tx.Rollback()

	var (
		oldSlotID string
		oldHoldID string
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(daily_slot_id, ''), COALESCE(slot_hold_id, '')
		FROM bookings
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, bookingID, workspaceID).Scan(&oldSlotID, &oldHoldID); err != nil {
		return Booking{}, err
	}

	var (
		startsAt time.Time
		endsAt   time.Time
		status   DailySlotStatus
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT starts_at, ends_at, status
		FROM daily_slots
		WHERE id = $1 AND workspace_id = $2
		FOR UPDATE
	`, dailySlotID, workspaceID).Scan(&startsAt, &endsAt, &status); err != nil {
		return Booking{}, err
	}
	if dailySlotID != oldSlotID && status != DailySlotFree {
		return Booking{}, errors.New("slot unavailable")
	}

	if oldHoldID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE id = $1 AND workspace_id = $2`, oldHoldID, workspaceID); err != nil {
			return Booking{}, err
		}
	}
	if oldSlotID != "" && oldSlotID != dailySlotID {
		if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, oldSlotID); err != nil {
			return Booking{}, err
		}
		if err := r.freeSlotIfUnusedTx(ctx, tx, workspaceID, oldSlotID, bookingID); err != nil {
			return Booking{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE bookings
		SET daily_slot_id = $3,
			slot_hold_id = NULL,
			starts_at = $4,
			ends_at = $5,
			amount_cents = $6,
			status = 'confirmed',
			notes = $7
		WHERE id = $1 AND workspace_id = $2
	`, bookingID, workspaceID, dailySlotID, startsAt, endsAt, amount*100, notes); err != nil {
		return Booking{}, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM slot_holds WHERE workspace_id = $1 AND daily_slot_id = $2`, workspaceID, dailySlotID); err != nil {
		return Booking{}, err
	}
	if _, err := r.syncSlotsTx(ctx, tx, workspaceID, oldSlotID, dailySlotID); err != nil {
		return Booking{}, err
	}

	if err := tx.Commit(); err != nil {
		return Booking{}, err
	}
	return r.Booking(ctx, workspaceID, bookingID)
}

func (r *Repository) Booking(ctx context.Context, workspaceID, bookingID string) (Booking, error) {
	var booking Booking
	if err := r.db.QueryRowContext(ctx, `
		SELECT b.id, b.workspace_id, b.customer_id, c.name, COALESCE(b.daily_slot_id, ''), b.starts_at, b.ends_at, COALESCE(b.amount_cents, 0) / 100, b.status, b.source, b.notes
		FROM bookings b
		JOIN customers c ON c.id = b.customer_id
		WHERE b.id = $1 AND b.workspace_id = $2
	`, bookingID, workspaceID).Scan(
		&booking.ID, &booking.WorkspaceID, &booking.CustomerID, &booking.CustomerName, &booking.DailySlotID, &booking.StartsAt, &booking.EndsAt, &booking.Amount, &booking.Status, &booking.Source, &booking.Notes,
	); err != nil {
		return Booking{}, err
	}
	return booking, nil
}

func (r *Repository) Bookings(ctx context.Context, workspaceID, statusFilter string) ([]Booking, error) {
	query := `
		SELECT b.id, b.workspace_id, b.customer_id, c.name, COALESCE(b.daily_slot_id, ''), b.starts_at, b.ends_at, COALESCE(b.amount_cents, 0) / 100, b.status, b.source, b.notes
		FROM bookings b
		JOIN customers c ON c.id = b.customer_id
		WHERE b.workspace_id = $1
	`
	args := []any{workspaceID}
	if statusFilter != "" && statusFilter != "all" {
		query += ` AND b.status = $2`
		args = append(args, statusFilter)
	}
	query += ` ORDER BY b.starts_at ASC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Booking
	for rows.Next() {
		var booking Booking
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
		); err != nil {
			return nil, err
		}
		items = append(items, booking)
	}
	return items, rows.Err()
}

func (r *Repository) Reviews(ctx context.Context, workspaceID string) ([]Review, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, customer_id, COALESCE(booking_id, ''), rating, body, status, created_at
		FROM reviews
		WHERE workspace_id = $1
		ORDER BY created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Review
	for rows.Next() {
		var item Review
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.CustomerID, &item.BookingID, &item.Rating, &item.Text, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Review(ctx context.Context, workspaceID, reviewID string) (Review, error) {
	var review Review
	err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, customer_id, booking_id, rating, text, status, created_at
		FROM reviews
		WHERE workspace_id = $1 AND id = $2
	`, workspaceID, reviewID).Scan(
		&review.ID,
		&review.WorkspaceID,
		&review.CustomerID,
		&review.BookingID,
		&review.Rating,
		&review.Text,
		&review.Status,
		&review.CreatedAt,
	)
	return review, err
}

func (r *Repository) UpdateReviewStatus(ctx context.Context, workspaceID, reviewID string, status ReviewStatus) (Review, error) {
	if _, err := r.db.ExecContext(ctx, `UPDATE reviews SET status = $3 WHERE id = $1 AND workspace_id = $2`, reviewID, workspaceID, status); err != nil {
		return Review{}, err
	}
	var review Review
	if err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, customer_id, COALESCE(booking_id, ''), rating, body, status, created_at
		FROM reviews
		WHERE id = $1 AND workspace_id = $2
	`, reviewID, workspaceID).Scan(
		&review.ID, &review.WorkspaceID, &review.CustomerID, &review.BookingID, &review.Rating, &review.Text, &review.Status, &review.CreatedAt,
	); err != nil {
		return Review{}, err
	}
	return review, nil
}

func (r *Repository) Analytics(ctx context.Context, workspaceID string) (AnalyticsOverview, error) {
	var overview AnalyticsOverview
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(revenue_cents / 100), 0), COALESCE(MAX(confirmation_rate), 0), COALESCE(MAX(no_show_rate), 0), COALESCE(MAX(repeat_bookings), 0), COALESCE(MAX(conversation_to_booking), 0)
		FROM analytics_daily
		WHERE workspace_id = $1
	`, workspaceID).Scan(&overview.Revenue, &overview.ConfirmationRate, &overview.NoShowRate, &overview.RepeatBookings, &overview.ConversationToBooking); err != nil {
		return AnalyticsOverview{}, err
	}
	return overview, nil
}

func (r *Repository) ChannelAccounts(ctx context.Context, workspaceID string) ([]ChannelAccount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id,
			workspace_id,
			provider,
			COALESCE(channel_kind, CASE WHEN provider = 'telegram' THEN 'telegram_client' WHEN provider = 'whatsapp' THEN 'whatsapp_twilio' ELSE provider END),
			COALESCE(account_scope, 'workspace'),
			account_name,
			connected,
			COALESCE(is_enabled, TRUE),
			external_account_id,
			COALESCE(bot_username, ''),
			COALESCE(webhook_secret, ''),
			COALESCE(bot_token_encrypted, '')
		FROM channel_accounts
		WHERE workspace_id = $1
		  AND revoked_at IS NULL
		  AND COALESCE(account_scope, 'workspace') = 'workspace'
		  AND COALESCE(channel_kind, CASE WHEN provider = 'telegram' THEN 'telegram_client' WHEN provider = 'whatsapp' THEN 'whatsapp_twilio' ELSE provider END) IN ('telegram_client', 'whatsapp_twilio')
		ORDER BY channel_kind ASC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []ChannelAccount
	for rows.Next() {
		var account ChannelAccount
		if err := rows.Scan(&account.ID, &account.WorkspaceID, &account.Provider, &account.ChannelKind, &account.AccountScope, &account.Name, &account.Connected, &account.IsEnabled, &account.AccountID, &account.BotUsername, &account.WebhookSecret, &account.EncryptedToken); err != nil {
			return nil, err
		}
		account.Channel = account.Provider
		account.TokenConfigured = account.EncryptedToken != ""
		account.WebhookURL = webhookURLForAccount(account)
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *Repository) UpdateChannel(ctx context.Context, workspaceID string, provider ChannelProvider, connected bool, name string) (ChannelAccount, error) {
	channelKind := defaultChannelKind(provider)
	if _, err := r.db.ExecContext(ctx, `
		UPDATE channel_accounts
		SET connected = $3,
			is_enabled = $3,
			account_name = COALESCE(NULLIF($4, ''), account_name)
		WHERE workspace_id = $1
		  AND channel_kind = $2
		  AND COALESCE(account_scope, 'workspace') = 'workspace'
		  AND revoked_at IS NULL
	`, workspaceID, channelKind, connected, name); err != nil {
		return ChannelAccount{}, err
	}
	return r.ChannelAccountByKind(ctx, workspaceID, channelKind)
}

func (r *Repository) BotConfig(ctx context.Context, workspaceID string) (BotConfig, []FAQItem, error) {
	var config BotConfig
	if err := r.db.QueryRowContext(ctx, `
		SELECT workspace_id, auto_reply, handoff_enabled, tone
		FROM bot_configs
		WHERE workspace_id = $1
	`, workspaceID).Scan(&config.WorkspaceID, &config.AutoReply, &config.HandoffEnabled, &config.Tone); err != nil {
		return BotConfig{}, nil, err
	}
	config.WelcomeMessage = defaultWelcomeMessage(config)
	config.HandoffMessage = defaultHandoffMessage(config)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workspace_id, question, answer
		FROM faq_items
		WHERE workspace_id = $1
		ORDER BY created_at ASC
	`, workspaceID)
	if err != nil {
		return BotConfig{}, nil, err
	}
	defer rows.Close()
	var faqItems []FAQItem
	for rows.Next() {
		var item FAQItem
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.Question, &item.Answer); err != nil {
			return BotConfig{}, nil, err
		}
		faqItems = append(faqItems, item)
	}
	config.FAQCount = len(faqItems)
	return config, faqItems, rows.Err()
}

func (r *Repository) UpdateBotConfig(ctx context.Context, workspaceID string, config BotConfig, faqItems []FAQItem) (BotConfig, []FAQItem, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return BotConfig{}, nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO bot_configs (workspace_id, auto_reply, handoff_enabled, tone, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (workspace_id) DO UPDATE
		SET auto_reply = EXCLUDED.auto_reply, handoff_enabled = EXCLUDED.handoff_enabled, tone = EXCLUDED.tone, updated_at = NOW()
	`, workspaceID, config.AutoReply, config.HandoffEnabled, config.Tone); err != nil {
		return BotConfig{}, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM faq_items WHERE workspace_id = $1`, workspaceID); err != nil {
		return BotConfig{}, nil, err
	}
	for i := range faqItems {
		if faqItems[i].ID == "" {
			faqItems[i].ID = newID("faq")
		}
		faqItems[i].WorkspaceID = workspaceID
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO faq_items (id, workspace_id, question, answer)
			VALUES ($1, $2, $3, $4)
		`, faqItems[i].ID, workspaceID, faqItems[i].Question, faqItems[i].Answer); err != nil {
			return BotConfig{}, nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return BotConfig{}, nil, err
	}
	config.WorkspaceID = workspaceID
	config.FAQCount = len(faqItems)
	config.WelcomeMessage = defaultWelcomeMessage(config)
	config.HandoffMessage = defaultHandoffMessage(config)
	return config, faqItems, nil
}

func (r *Repository) IngestWebhook(ctx context.Context, provider ChannelProvider, customerName, text string) (Conversation, Message, Customer, error) {
	account, err := r.ChannelAccountByKind(ctx, demoWorkspaceID, defaultChannelKind(provider))
	if err != nil {
		return Conversation{}, Message{}, Customer{}, err
	}
	result, err := r.ReceiveInboundMessage(ctx, InboundMessageInput{
		Provider:          provider,
		ChannelAccountID:  account.ID,
		ExternalChatID:    normalizedExternalID(provider, customerName),
		ExternalMessageID: newID("ext"),
		Text:              text,
		Timestamp:         time.Now().UTC(),
		Profile: InboundProfile{
			Name:     customerName,
			Username: strings.ToLower(strings.ReplaceAll(customerName, " ", "_")),
		},
	})
	if err != nil {
		return Conversation{}, Message{}, Customer{}, err
	}
	return result.Conversation, result.Message, result.Customer, nil
}

func (r *Repository) RefreshAnalytics(ctx context.Context, workspaceID string) error {
	var revenue, confirmationRate, noShowRate, repeatBookings, conversationToBooking int
	if err := r.db.QueryRowContext(ctx, `
		WITH booking_stats AS (
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE status IN ('confirmed', 'completed')) AS confirmed,
				COUNT(*) FILTER (WHERE status = 'cancelled') AS cancelled,
				COALESCE(SUM(amount_cents) FILTER (WHERE status IN ('confirmed', 'completed')), 0) AS revenue_cents,
				COUNT(*) FILTER (WHERE customer_id IN (
					SELECT customer_id FROM bookings WHERE workspace_id = $1 GROUP BY customer_id HAVING COUNT(*) > 1
				)) AS repeat_count
			FROM bookings
			WHERE workspace_id = $1
		),
		conversation_stats AS (
			SELECT COUNT(*) AS total_conversations FROM conversations WHERE workspace_id = $1
		)
		SELECT
			COALESCE(booking_stats.revenue_cents, 0),
			CASE WHEN booking_stats.total = 0 THEN 0 ELSE ROUND((booking_stats.confirmed::numeric / booking_stats.total::numeric) * 100) END,
			CASE WHEN booking_stats.total = 0 THEN 0 ELSE ROUND((booking_stats.cancelled::numeric / booking_stats.total::numeric) * 100) END,
			CASE WHEN booking_stats.total = 0 THEN 0 ELSE ROUND((booking_stats.repeat_count::numeric / booking_stats.total::numeric) * 100) END,
			CASE WHEN conversation_stats.total_conversations = 0 THEN 0 ELSE ROUND((booking_stats.total::numeric / conversation_stats.total_conversations::numeric) * 100) END
		FROM booking_stats, conversation_stats
	`, workspaceID).Scan(&revenue, &confirmationRate, &noShowRate, &repeatBookings, &conversationToBooking); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO analytics_daily (id, workspace_id, bucket_date, revenue_cents, confirmation_rate, no_show_rate, repeat_bookings, conversation_to_booking)
		VALUES ($1, $2, CURRENT_DATE, $3, $4, $5, $6, $7)
		ON CONFLICT (workspace_id, bucket_date) DO UPDATE
		SET revenue_cents = EXCLUDED.revenue_cents,
			confirmation_rate = EXCLUDED.confirmation_rate,
			no_show_rate = EXCLUDED.no_show_rate,
			repeat_bookings = EXCLUDED.repeat_bookings,
			conversation_to_booking = EXCLUDED.conversation_to_booking
	`, "anl_"+time.Now().UTC().Format("20060102"), workspaceID, revenue, confirmationRate, noShowRate, repeatBookings, conversationToBooking)
	return err
}

func normalizedExternalID(provider ChannelProvider, customerName string) string {
	normalized := strings.ToLower(strings.TrimSpace(customerName))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	if normalized == "" {
		normalized = "anonymous"
	}
	return fmt.Sprintf("%s:%s", provider, normalized)
}
