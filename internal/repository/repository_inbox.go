package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/vital/rendycrm-app/internal/domain"
)

func (r *Repository) ChannelAccountByID(ctx context.Context, channelAccountID string) (ChannelAccount, error) {
	return r.channelAccountByID(ctx, r.db, channelAccountID)
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *Repository) channelAccountByID(ctx context.Context, q queryRower, channelAccountID string) (ChannelAccount, error) {
	var account ChannelAccount
	if err := q.QueryRowContext(ctx, `
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
		WHERE id = $1
		  AND revoked_at IS NULL
	`, channelAccountID).Scan(
		&account.ID,
		&account.WorkspaceID,
		&account.Provider,
		&account.ChannelKind,
		&account.AccountScope,
		&account.Name,
		&account.Connected,
		&account.IsEnabled,
		&account.AccountID,
		&account.BotUsername,
		&account.WebhookSecret,
		&account.EncryptedToken,
	); err != nil {
		return ChannelAccount{}, err
	}
	account.Channel = account.Provider
	account.TokenConfigured = strings.TrimSpace(account.EncryptedToken) != ""
	account.WebhookURL = webhookURLForAccount(account)
	return account, nil
}

func (r *Repository) channelAccountByIDTx(ctx context.Context, tx *sql.Tx, channelAccountID string) (ChannelAccount, error) {
	return r.channelAccountByID(ctx, tx, channelAccountID)
}

func (r *Repository) conversationByExternalChatTx(ctx context.Context, tx *sql.Tx, workspaceID string, provider ChannelProvider, externalChatID string) (Conversation, error) {
	var item Conversation
	err := tx.QueryRowContext(ctx, `
		SELECT
			c.id,
			c.workspace_id,
			c.customer_id,
			c.provider,
			c.external_chat_id,
			cust.name,
			c.status,
			COALESCE(c.assigned_user_id, ''),
			c.unread_count,
			c.ai_summary,
			c.intent,
			c.last_message_text,
			COALESCE(c.last_inbound_at, 'epoch'::timestamptz),
			COALESCE(c.last_outbound_at, 'epoch'::timestamptz),
			c.updated_at
		FROM conversations c
		JOIN customers cust ON cust.id = c.customer_id
		WHERE c.workspace_id = $1
		  AND c.provider = $2
		  AND c.external_chat_id = $3
		  AND c.status <> 'closed'
		ORDER BY c.updated_at DESC
		LIMIT 1
		FOR UPDATE
	`, workspaceID, provider, externalChatID).Scan(
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
	)
	if err != nil {
		return Conversation{}, err
	}
	item.Channel = item.Provider
	item.AssignedOperatorID = item.AssignedUserID
	return item, nil
}

func (r *Repository) upsertCustomerIdentityTx(ctx context.Context, tx *sql.Tx, workspaceID string, provider ChannelProvider, externalChatID string, profile InboundProfile) (Customer, error) {
	var customerID string
	err := tx.QueryRowContext(ctx, `
		SELECT cci.customer_id
		FROM customer_channel_identities cci
		WHERE cci.workspace_id = $1
		  AND cci.provider = $2
		  AND cci.external_id = $3
		FOR UPDATE
	`, workspaceID, provider, externalChatID).Scan(&customerID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return Customer{}, err
		}
		customerID = newID("cus")
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			name = "Новый клиент"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO customers (id, workspace_id, name, notes)
			VALUES ($1, $2, $3, 'Created from channel inbound')
		`, customerID, workspaceID, name); err != nil {
			return Customer{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO customer_channel_identities (id, customer_id, workspace_id, provider, external_id, username)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, newID("cci"), customerID, workspaceID, provider, externalChatID, strings.TrimSpace(profile.Username)); err != nil {
			return Customer{}, err
		}
		if phone := strings.TrimSpace(profile.Phone); phone != "" {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO customer_contacts (id, customer_id, type, value, is_primary)
				VALUES ($1, $2, 'phone', $3, TRUE)
				ON CONFLICT (customer_id, type, value) DO NOTHING
			`, newID("cct"), customerID, phone); err != nil {
				return Customer{}, err
			}
		}
	} else {
		if strings.TrimSpace(profile.Name) != "" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE customers
				SET name = $3
				WHERE id = $1 AND workspace_id = $2
			`, customerID, workspaceID, strings.TrimSpace(profile.Name)); err != nil {
				return Customer{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE customer_channel_identities
			SET username = COALESCE(NULLIF($5, ''), username)
			WHERE customer_id = $1 AND workspace_id = $2 AND provider = $3 AND external_id = $4
		`, customerID, workspaceID, provider, externalChatID, strings.TrimSpace(profile.Username)); err != nil {
			return Customer{}, err
		}
		if phone := strings.TrimSpace(profile.Phone); phone != "" {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO customer_contacts (id, customer_id, type, value, is_primary)
				VALUES ($1, $2, 'phone', $3, TRUE)
				ON CONFLICT (customer_id, type, value) DO NOTHING
			`, newID("cct"), customerID, phone); err != nil {
				return Customer{}, err
			}
		}
	}
	return r.customerTx(ctx, tx, workspaceID, customerID)
}

func (r *Repository) customerTx(ctx context.Context, tx *sql.Tx, workspaceID, customerID string) (Customer, error) {
	var customer Customer
	if err := tx.QueryRowContext(ctx, `
		SELECT
			c.id,
			c.workspace_id,
			c.name,
			COALESCE(MAX(CASE WHEN cc.type = 'phone' AND cc.is_primary THEN cc.value END), ''),
			COALESCE(MAX(CASE WHEN cc.type = 'email' AND cc.is_primary THEN cc.value END), ''),
			c.notes,
			COALESCE(MAX(b.starts_at), NOW() - INTERVAL '14 days'),
			COUNT(DISTINCT b.id),
			COALESCE(MAX(cci.provider), '')
		FROM customers c
		LEFT JOIN customer_contacts cc ON cc.customer_id = c.id
		LEFT JOIN bookings b ON b.customer_id = c.id AND b.workspace_id = c.workspace_id
		LEFT JOIN customer_channel_identities cci ON cci.customer_id = c.id
		WHERE c.workspace_id = $1 AND c.id = $2
		GROUP BY c.id, c.workspace_id, c.name, c.notes
	`, workspaceID, customerID).Scan(
		&customer.ID,
		&customer.WorkspaceID,
		&customer.Name,
		&customer.Phone,
		&customer.Email,
		&customer.Notes,
		&customer.LastVisitAt,
		&customer.BookingCount,
		&customer.PreferredChannel,
	); err != nil {
		return Customer{}, err
	}
	identityRows, err := tx.QueryContext(ctx, `
		SELECT id, provider, external_id, username
		FROM customer_channel_identities
		WHERE customer_id = $1 AND workspace_id = $2
		ORDER BY created_at ASC
	`, customerID, workspaceID)
	if err != nil {
		return Customer{}, err
	}
	defer identityRows.Close()
	for identityRows.Next() {
		var identity CustomerChannelIdentity
		if err := identityRows.Scan(&identity.ID, &identity.Provider, &identity.ExternalID, &identity.Username); err != nil {
			return Customer{}, err
		}
		identity.DisplayName = identity.Username
		customer.Channels = append(customer.Channels, identity)
	}
	return customer, identityRows.Err()
}

func (r *Repository) createConversationTx(ctx context.Context, tx *sql.Tx, targetWorkspaceID string, account ChannelAccount, customer Customer, externalChatID string, status ConversationStatus, intent AutomationIntent, now time.Time) (Conversation, error) {
	conversation := Conversation{
		ID:                 newID("cnv"),
		WorkspaceID:        targetWorkspaceID,
		CustomerID:         customer.ID,
		Provider:           account.Provider,
		Channel:            account.Provider,
		ExternalChatID:     externalChatID,
		Title:              customer.Name,
		Status:             status,
		AssignedUserID:     "",
		AssignedOperatorID: "",
		UnreadCount:        0,
		AISummary:          "",
		Intent:             intent,
		LastMessageText:    "",
		LastInboundAt:      now,
		LastOutboundAt:     time.Time{},
		UpdatedAt:          now,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversations (
			id, workspace_id, customer_id, channel_account_id, provider, external_chat_id,
			status, unread_count, ai_summary, intent, last_message_text, last_inbound_at, last_outbound_at, updated_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, '', $8, '', $9, NULL, $9, $9)
	`, conversation.ID, conversation.WorkspaceID, conversation.CustomerID, account.ID, account.Provider, externalChatID, conversation.Status, conversation.Intent, now); err != nil {
		return Conversation{}, err
	}
	return conversation, nil
}

func (r *Repository) SendBotReply(ctx context.Context, workspaceID, conversationID, text string, buttons []string, status ConversationStatus, intent AutomationIntent) (Conversation, Message, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Conversation{}, Message{}, err
	}
	defer tx.Rollback()

	targetConversation, account, err := r.outboundTargetTx(ctx, tx, workspaceID, conversationID)
	if err != nil {
		return Conversation{}, Message{}, err
	}
	now := time.Now().UTC()
	message := Message{
		ID:             newID("msg"),
		ConversationID: conversationID,
		Direction:      MessageOutbound,
		SenderType:     MessageSenderBot,
		Text:           text,
		Status:         string(MessageDeliveryQueued),
		DeliveryStatus: string(MessageDeliveryQueued),
		CreatedAt:      now,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, workspace_id, channel_account_id, external_message_id, direction, sender_type, body, delivery_status, dedup_key, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), '', $5, $6, $7, $8, $9, $10)
	`, message.ID, conversationID, workspaceID, account.ID, message.Direction, message.SenderType, message.Text, message.DeliveryStatus, message.ID, message.CreatedAt); err != nil {
		return Conversation{}, Message{}, err
	}
	if account.Provider == ChannelTelegram {
		payload := TelegramOutboundPayload{
			ChatID: targetConversation.ExternalChatID,
			Text:   text,
		}
		kind := OutboundKindTelegramSendText
		if len(buttons) > 0 {
			kind = OutboundKindTelegramSendInline
			payload.Buttons = make([]TelegramInlineButton, 0, len(buttons))
			for _, button := range buttons {
				payload.Buttons = append(payload.Buttons, TelegramInlineButton{
					Text:         button,
					CallbackData: button,
				})
			}
		}
		if _, err := r.enqueueOutboundMessageTx(ctx, tx, OutboundMessage{
			WorkspaceID:      workspaceID,
			Channel:          account.Provider,
			ChannelKind:      account.ChannelKind,
			ChannelAccountID: account.ID,
			ConversationID:   conversationID,
			MessageID:        message.ID,
			Kind:             kind,
		}, payload); err != nil {
			return Conversation{}, Message{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET status = $3,
			intent = $4,
			last_message_text = $5,
			last_outbound_at = $6,
			updated_at = $6
		WHERE id = $1 AND workspace_id = $2
	`, conversationID, workspaceID, status, intent, text, now); err != nil {
		return Conversation{}, Message{}, err
	}
	if err := tx.Commit(); err != nil {
		return Conversation{}, Message{}, err
	}
	conversation, _, _, err := r.ConversationDetail(ctx, workspaceID, conversationID)
	return conversation, message, err
}

func (r *Repository) ReceiveInboundMessage(ctx context.Context, input InboundMessageInput) (InboxReceiveResult, error) {
	account, err := r.ChannelAccountByID(ctx, input.ChannelAccountID)
	if err != nil {
		return InboxReceiveResult{}, err
	}
	return r.receiveInboundMessageForWorkspace(ctx, account.WorkspaceID, account, input)
}

func (r *Repository) ReceiveInboundMessageForWorkspace(ctx context.Context, workspaceID string, input InboundMessageInput) (InboxReceiveResult, error) {
	account, err := r.ChannelAccountByID(ctx, input.ChannelAccountID)
	if err != nil {
		return InboxReceiveResult{}, err
	}
	return r.receiveInboundMessageForWorkspace(ctx, workspaceID, account, input)
}

func (r *Repository) receiveInboundMessageForWorkspace(ctx context.Context, targetWorkspaceID string, account ChannelAccount, input InboundMessageInput) (InboxReceiveResult, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return InboxReceiveResult{}, err
	}
	defer tx.Rollback()
	if account.Provider != input.Provider {
		return InboxReceiveResult{}, errors.New("channel provider mismatch")
	}
	if !account.Connected {
		return InboxReceiveResult{}, errors.New("channel is disconnected")
	}

	customer, err := r.upsertCustomerIdentityTx(ctx, tx, targetWorkspaceID, input.Provider, input.ExternalChatID, input.Profile)
	if err != nil {
		return InboxReceiveResult{}, err
	}

	intent := detectIntent(input.Text)
	status := ConversationNew

	conversation, err := r.conversationByExternalChatTx(ctx, tx, targetWorkspaceID, input.Provider, input.ExternalChatID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return InboxReceiveResult{}, err
		}
		conversation, err = r.createConversationTx(ctx, tx, targetWorkspaceID, account, customer, input.ExternalChatID, status, intent, input.Timestamp)
		if err != nil {
			return InboxReceiveResult{}, err
		}
	}

	message := Message{
		ID:             newID("msg"),
		ConversationID: conversation.ID,
		Direction:      MessageInbound,
		SenderType:     MessageSenderCustomer,
		Text:           strings.TrimSpace(input.Text),
		Status:         "received",
		DeliveryStatus: "received",
		ExternalID:     strings.TrimSpace(input.ExternalMessageID),
		CreatedAt:      input.Timestamp,
	}
	dedupKey := message.ID
	if message.ExternalID != "" {
		dedupKey = fmt.Sprintf("%s:%s", account.ID, message.ExternalID)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO messages (
			id, conversation_id, workspace_id, channel_account_id, external_message_id,
			direction, sender_type, body, delivery_status, dedup_key, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (conversation_id, dedup_key) DO NOTHING
	`, message.ID, conversation.ID, conversation.WorkspaceID, account.ID, message.ExternalID, message.Direction, message.SenderType, message.Text, message.DeliveryStatus, dedupKey, message.CreatedAt)
	if err != nil {
		return InboxReceiveResult{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		conversation, _, customer, err := r.ConversationDetail(ctx, conversation.WorkspaceID, conversation.ID)
		if err != nil {
			return InboxReceiveResult{}, err
		}
		return InboxReceiveResult{Conversation: conversation, Customer: customer, Stored: false}, nil
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET customer_id = $3,
			status = $4,
			intent = $5,
			last_message_text = $6,
			unread_count = unread_count + 1,
			last_inbound_at = $7,
			updated_at = $7
		WHERE id = $1 AND workspace_id = $2
	`, conversation.ID, conversation.WorkspaceID, customer.ID, status, intent, message.Text, message.CreatedAt); err != nil {
		return InboxReceiveResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return InboxReceiveResult{}, err
	}

	conversation, messages, customer, err := r.ConversationDetail(ctx, conversation.WorkspaceID, conversation.ID)
	if err != nil {
		return InboxReceiveResult{}, err
	}
	conversation.Intent = intent

	autoResponses, finalConversation, err := r.runAutomation(ctx, account, conversation, messages)
	if err != nil {
		return InboxReceiveResult{}, err
	}
	if finalConversation.ID != "" {
		conversation = finalConversation
	}

	return InboxReceiveResult{
		Conversation: conversation,
		Message:      message,
		Customer:     customer,
		Responses:    autoResponses,
		Stored:       true,
	}, nil
}

func (r *Repository) runAutomation(ctx context.Context, account ChannelAccount, conversation Conversation, messages []Message) ([]BotOutboundMessage, Conversation, error) {
	workspaceID := conversation.WorkspaceID
	config, faqItems, err := r.BotConfig(ctx, workspaceID)
	if err != nil {
		return nil, Conversation{}, err
	}
	if !config.AutoReply {
		updated, err := r.UpdateConversationAutomation(ctx, workspaceID, conversation.ID, ConversationNew, conversation.Intent)
		return nil, updated, err
	}

	decision := automationDecision{
		Intent: conversation.Intent,
		Status: ConversationNew,
	}
	if countRecentBotMessages(messages, 24*time.Hour) >= 2 && config.HandoffEnabled {
		decision.Intent = IntentHumanRequest
		decision.Status = ConversationHuman
		decision.Response = defaultHandoffMessage(config)
		decision.ShouldReply = true
	} else {
		decision = r.decideAutomationResponse(ctx, workspaceID, conversation, messages, faqItems, config)
	}

	updated, err := r.UpdateConversationAutomation(ctx, workspaceID, conversation.ID, decision.Status, decision.Intent)
	if err != nil {
		return nil, Conversation{}, err
	}
	if !decision.ShouldReply {
		return nil, updated, nil
	}
	replyConversation, _, err := r.SendBotReply(ctx, workspaceID, conversation.ID, decision.Response, decision.Buttons, decision.Status, decision.Intent)
	if err != nil {
		return nil, Conversation{}, err
	}
	return []BotOutboundMessage{{
		ChatID:         conversation.ExternalChatID,
		Text:           decision.Response,
		Buttons:        decision.Buttons,
		Channel:        string(account.Provider),
		ConversationID: conversation.ID,
	}}, replyConversation, nil
}

func defaultWelcomeMessage(config BotConfig) string {
	if strings.TrimSpace(config.WelcomeMessage) != "" {
		return config.WelcomeMessage
	}
	return "Здравствуйте! Помогу записаться или быстро отвечу на вопрос."
}

func defaultHandoffMessage(config BotConfig) string {
	if strings.TrimSpace(config.HandoffMessage) != "" {
		return config.HandoffMessage
	}
	return "Передаю диалог оператору. Он ответит здесь же."
}

func (r *Repository) decideAutomationResponse(ctx context.Context, workspaceID string, conversation Conversation, messages []Message, faqItems []FAQItem, config BotConfig) automationDecision {
	lastText := conversation.LastMessageText
	if conversation.LastInboundAt.IsZero() && len(messages) > 0 {
		lastText = messages[len(messages)-1].Text
	}
	intent := detectIntent(lastText)
	if faq := matchFAQ(lastText, faqItems); faq != nil {
		return automationDecision{
			Intent:      IntentFAQ,
			Status:      ConversationAuto,
			Response:    faq.Answer,
			ShouldReply: true,
		}
	}
	switch intent {
	case IntentHumanRequest:
		if config.HandoffEnabled {
			return automationDecision{
				Intent:      IntentHumanRequest,
				Status:      ConversationHuman,
				Response:    defaultHandoffMessage(config),
				ShouldReply: true,
			}
		}
	case IntentBookingRequest, IntentAvailabilityQuestion, IntentReschedule:
		slots, err := r.AvailableDaySlots(ctx, workspaceID, time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 14))
		if err == nil {
			response, buttons := formatSlotsForBot(slots)
			status := ConversationAuto
			if len(slots) == 0 && config.HandoffEnabled {
				status = ConversationHuman
			}
			return automationDecision{
				Intent:      intent,
				Status:      status,
				Response:    response,
				Buttons:     buttons,
				ShouldReply: true,
			}
		}
	case IntentCancel:
		if config.HandoffEnabled {
			return automationDecision{
				Intent:      IntentCancel,
				Status:      ConversationHuman,
				Response:    "Понял, передаю запрос на отмену оператору.",
				ShouldReply: true,
			}
		}
	case IntentPriceQuestion:
		if faq := matchFAQ("цена стоимость прайс", faqItems); faq != nil {
			return automationDecision{
				Intent:      IntentPriceQuestion,
				Status:      ConversationAuto,
				Response:    faq.Answer,
				ShouldReply: true,
			}
		}
	}
	if config.HandoffEnabled {
		return automationDecision{
			Intent:      intent,
			Status:      ConversationHuman,
			Response:    defaultHandoffMessage(config),
			ShouldReply: true,
		}
	}
	return automationDecision{
		Intent:      intent,
		Status:      ConversationAuto,
		Response:    defaultWelcomeMessage(config),
		ShouldReply: true,
	}
}

func (r *Repository) UpdateConversationAutomation(ctx context.Context, workspaceID, conversationID string, status ConversationStatus, intent AutomationIntent) (Conversation, error) {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE conversations
		SET status = $3, intent = $4, updated_at = NOW()
		WHERE id = $1 AND workspace_id = $2
	`, conversationID, workspaceID, status, intent); err != nil {
		return Conversation{}, err
	}
	conversation, _, _, err := r.ConversationDetail(ctx, workspaceID, conversationID)
	return conversation, err
}

func (r *Repository) CreateOperatorLinkCode(ctx context.Context, workspaceID, userID, botUsername string) (OperatorBotLinkCode, error) {
	code := newID("link")
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO operator_bot_link_codes (id, user_id, workspace_id, code, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, newID("obl"), userID, workspaceID, code, expiresAt); err != nil {
		return OperatorBotLinkCode{}, err
	}
	return OperatorBotLinkCode{
		ID:          "",
		UserID:      userID,
		WorkspaceID: workspaceID,
		Code:        code,
		ExpiresAt:   expiresAt,
		DeepLink:    fmt.Sprintf("https://t.me/%s?start=%s", strings.TrimPrefix(botUsername, "@"), code),
	}, nil
}

func (r *Repository) OperatorBotSettings(ctx context.Context, workspaceID, userID, botUsername, publicBaseURL string) (OperatorBotSettings, error) {
	settings := OperatorBotSettings{
		BotUsername:        botUsername,
		OperatorWebhookURL: strings.TrimRight(publicBaseURL, "/") + "/webhooks/telegram/operator",
	}
	if account, err := r.ChannelAccountByKind(ctx, workspaceID, ChannelKindTelegramOperator); err == nil {
		if strings.TrimSpace(account.BotUsername) != "" {
			settings.BotUsername = account.BotUsername
		}
		settings.TokenConfigured = account.TokenConfigured
	}
	var binding OperatorBotBinding
	err := r.db.QueryRowContext(ctx, `
		SELECT user_id, workspace_id, telegram_user_id, telegram_chat_id, linked_at, is_active
		FROM operator_bot_bindings
		WHERE user_id = $1 AND workspace_id = $2 AND is_active = TRUE
	`, userID, workspaceID).Scan(&binding.UserID, &binding.WorkspaceID, &binding.TelegramUserID, &binding.TelegramChatID, &binding.LinkedAt, &binding.IsActive)
	if err == nil {
		settings.Binding = &binding
	} else if !errors.Is(err, sql.ErrNoRows) {
		return OperatorBotSettings{}, err
	}

	var pending OperatorBotLinkCode
	err = r.db.QueryRowContext(ctx, `
		SELECT id, user_id, workspace_id, code, expires_at
		FROM operator_bot_link_codes
		WHERE user_id = $1
		  AND workspace_id = $2
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`, userID, workspaceID).Scan(&pending.ID, &pending.UserID, &pending.WorkspaceID, &pending.Code, &pending.ExpiresAt)
	if err == nil {
		pending.DeepLink = fmt.Sprintf("https://t.me/%s?start=%s", strings.TrimPrefix(settings.BotUsername, "@"), pending.Code)
		settings.PendingLink = &pending
	} else if !errors.Is(err, sql.ErrNoRows) {
		return OperatorBotSettings{}, err
	}
	return settings, nil
}

func (r *Repository) LinkOperatorTelegram(ctx context.Context, code, telegramUserID, telegramChatID string) (OperatorBotBinding, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return OperatorBotBinding{}, err
	}
	defer tx.Rollback()

	var userID, workspaceID string
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, workspace_id
		FROM operator_bot_link_codes
		WHERE code = $1
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		FOR UPDATE
	`, code).Scan(&userID, &workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OperatorBotBinding{}, errors.New("link code expired or invalid")
		}
		return OperatorBotBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE operator_bot_link_codes
		SET consumed_at = NOW()
		WHERE code = $1
	`, code); err != nil {
		return OperatorBotBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO operator_bot_bindings (user_id, workspace_id, telegram_user_id, telegram_chat_id, linked_at, is_active)
		VALUES ($1, $2, $3, $4, NOW(), TRUE)
		ON CONFLICT (user_id) DO UPDATE
		SET workspace_id = EXCLUDED.workspace_id,
			telegram_user_id = EXCLUDED.telegram_user_id,
			telegram_chat_id = EXCLUDED.telegram_chat_id,
			linked_at = NOW(),
			is_active = TRUE
	`, userID, workspaceID, telegramUserID, telegramChatID); err != nil {
		return OperatorBotBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return OperatorBotBinding{}, err
	}
	return OperatorBotBinding{
		UserID:         userID,
		WorkspaceID:    workspaceID,
		TelegramUserID: telegramUserID,
		TelegramChatID: telegramChatID,
		LinkedAt:       time.Now().UTC(),
		IsActive:       true,
	}, nil
}

func (r *Repository) UnlinkOperatorTelegram(ctx context.Context, workspaceID, userID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE operator_bot_bindings
		SET is_active = FALSE
		WHERE user_id = $1 AND workspace_id = $2
	`, userID, workspaceID)
	return err
}

func (r *Repository) ActiveOperatorBindingByTelegramChat(ctx context.Context, telegramChatID string) (OperatorBotBinding, error) {
	var binding OperatorBotBinding
	if err := r.db.QueryRowContext(ctx, `
		SELECT user_id, workspace_id, telegram_user_id, telegram_chat_id, linked_at, is_active
		FROM operator_bot_bindings
		WHERE telegram_chat_id = $1 AND is_active = TRUE
	`, telegramChatID).Scan(&binding.UserID, &binding.WorkspaceID, &binding.TelegramUserID, &binding.TelegramChatID, &binding.LinkedAt, &binding.IsActive); err != nil {
		return OperatorBotBinding{}, err
	}
	return binding, nil
}

func (r *Repository) SaveBotSession(ctx context.Context, session BotSession, payload any) (BotSession, error) {
	raw := []byte("{}")
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return BotSession{}, err
		}
		raw = encoded
	}
	if session.ID == "" {
		session.ID = newID("bss")
	}
	if session.BotKind == "" {
		session.BotKind = ChannelKindTelegramOperator
	}
	if session.ExpiresAt.IsZero() {
		session.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	session.UpdatedAt = time.Now().UTC()
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO bot_sessions (id, workspace_id, bot_kind, scope, actor_type, actor_id, state, payload_json, expires_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10)
		ON CONFLICT (workspace_id, scope, actor_type, actor_id) DO UPDATE
		SET bot_kind = EXCLUDED.bot_kind,
			state = EXCLUDED.state,
			payload_json = EXCLUDED.payload_json,
			expires_at = EXCLUDED.expires_at,
			updated_at = EXCLUDED.updated_at
	`, session.ID, session.WorkspaceID, session.BotKind, session.Scope, session.ActorType, session.ActorID, session.State, string(raw), session.ExpiresAt, session.UpdatedAt); err != nil {
		return BotSession{}, err
	}
	session.Payload = string(raw)
	return session, nil
}

func (r *Repository) LoadBotSession(ctx context.Context, workspaceID string, scope BotSessionScope, actorType BotSessionActorType, actorID string) (BotSession, error) {
	var session BotSession
	if err := r.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, bot_kind, scope, actor_type, actor_id, state, payload_json::text, expires_at, updated_at
		FROM bot_sessions
		WHERE workspace_id = $1 AND scope = $2 AND actor_type = $3 AND actor_id = $4 AND expires_at > NOW()
	`, workspaceID, scope, actorType, actorID).Scan(
		&session.ID,
		&session.WorkspaceID,
		&session.BotKind,
		&session.Scope,
		&session.ActorType,
		&session.ActorID,
		&session.State,
		&session.Payload,
		&session.ExpiresAt,
		&session.UpdatedAt,
	); err != nil {
		return BotSession{}, err
	}
	return session, nil
}

func (r *Repository) DeleteBotSession(ctx context.Context, workspaceID string, scope BotSessionScope, actorType BotSessionActorType, actorID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM bot_sessions
		WHERE workspace_id = $1 AND scope = $2 AND actor_type = $3 AND actor_id = $4
	`, workspaceID, scope, actorType, actorID)
	return err
}
