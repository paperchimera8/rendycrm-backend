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

func defaultChannelKind(provider ChannelProvider) ChannelKind {
	switch provider {
	case ChannelTelegram:
		return ChannelKindTelegramClient
	case ChannelWhatsApp:
		return ChannelKindWhatsAppTwilio
	default:
		return ChannelKind(provider)
	}
}

func webhookURLForAccount(account ChannelAccount) string {
	switch account.ChannelKind {
	case ChannelKindTelegramClient:
		return fmt.Sprintf("/webhooks/telegram/client/%s/%s", account.ID, account.WebhookSecret)
	case ChannelKindWhatsAppTwilio:
		return fmt.Sprintf("/webhooks/whatsapp/twilio/%s/%s", account.ID, account.WebhookSecret)
	default:
		return "/webhooks/" + string(account.Provider)
	}
}

func (r *Repository) ChannelAccountByKind(ctx context.Context, workspaceID string, kind ChannelKind) (ChannelAccount, error) {
	var account ChannelAccount
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			id,
			workspace_id,
			provider,
			channel_kind,
			COALESCE(account_scope, 'workspace'),
			account_name,
			connected,
			is_enabled,
			external_account_id,
			COALESCE(bot_username, ''),
			COALESCE(webhook_secret, ''),
			COALESCE(bot_token_encrypted, '')
		FROM channel_accounts
		WHERE workspace_id = $1
		  AND channel_kind = $2
		  AND revoked_at IS NULL
		LIMIT 1
	`, workspaceID, kind).Scan(
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
	account.TokenConfigured = account.EncryptedToken != ""
	account.WebhookURL = webhookURLForAccount(account)
	return account, nil
}

func (r *Repository) ChannelAccountByWebhookSecret(ctx context.Context, kind ChannelKind, secret string) (ChannelAccount, error) {
	var account ChannelAccount
	if err := r.db.QueryRowContext(ctx, `
		SELECT
			id,
			workspace_id,
			provider,
			channel_kind,
			COALESCE(account_scope, 'workspace'),
			account_name,
			connected,
			is_enabled,
			external_account_id,
			COALESCE(bot_username, ''),
			COALESCE(webhook_secret, ''),
			COALESCE(bot_token_encrypted, '')
		FROM channel_accounts
		WHERE channel_kind = $1
		  AND webhook_secret = $2
		  AND revoked_at IS NULL
		LIMIT 1
	`, kind, secret).Scan(
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
	account.TokenConfigured = account.EncryptedToken != ""
	account.WebhookURL = webhookURLForAccount(account)
	return account, nil
}

func (r *Repository) UpsertChannelAccount(ctx context.Context, account ChannelAccount) (ChannelAccount, error) {
	if account.ID == "" {
		account.ID = newID("cha")
	}
	if account.ChannelKind == "" {
		account.ChannelKind = defaultChannelKind(account.Provider)
	}
	if account.AccountScope == "" {
		account.AccountScope = ChannelAccountScopeWorkspace
	}
	if account.AccountID == "" {
		account.AccountID = string(account.ChannelKind)
	}
	if account.WebhookSecret == "" {
		account.WebhookSecret = newID("whsec")
	}
	if account.Name == "" {
		account.Name = string(account.ChannelKind)
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO channel_accounts (
			id, workspace_id, provider, channel_kind, account_scope, account_name, connected, is_enabled,
			external_account_id, webhook_secret, bot_username, bot_token_encrypted
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE
		SET account_scope = EXCLUDED.account_scope,
			account_name = EXCLUDED.account_name,
			connected = EXCLUDED.connected,
			is_enabled = EXCLUDED.is_enabled,
			external_account_id = EXCLUDED.external_account_id,
			webhook_secret = EXCLUDED.webhook_secret,
			bot_username = EXCLUDED.bot_username,
			bot_token_encrypted = CASE
				WHEN EXCLUDED.bot_token_encrypted = '' THEN channel_accounts.bot_token_encrypted
				ELSE EXCLUDED.bot_token_encrypted
			END,
			revoked_at = NULL
	`, account.ID, account.WorkspaceID, account.Provider, account.ChannelKind, account.AccountScope, account.Name, account.Connected, account.IsEnabled, account.AccountID, account.WebhookSecret, account.BotUsername, account.EncryptedToken); err != nil {
		var existingID string
		if errLookup := r.db.QueryRowContext(ctx, `
			SELECT id
			FROM channel_accounts
			WHERE workspace_id = $1 AND channel_kind = $2 AND COALESCE(account_scope, 'workspace') = $3 AND revoked_at IS NULL
		`, account.WorkspaceID, account.ChannelKind, account.AccountScope).Scan(&existingID); errLookup != nil {
			return ChannelAccount{}, err
		}
		if _, errUpdate := r.db.ExecContext(ctx, `
			UPDATE channel_accounts
			SET account_scope = $3,
				account_name = $4,
				connected = $5,
				is_enabled = $6,
				external_account_id = $7,
				webhook_secret = $8,
				bot_username = $9,
				bot_token_encrypted = CASE WHEN $10 = '' THEN bot_token_encrypted ELSE $10 END,
				revoked_at = NULL
			WHERE id = $1 AND workspace_id = $2
		`, existingID, account.WorkspaceID, account.AccountScope, account.Name, account.Connected, account.IsEnabled, account.AccountID, account.WebhookSecret, account.BotUsername, account.EncryptedToken); errUpdate != nil {
			return ChannelAccount{}, errUpdate
		}
		return r.ChannelAccountByID(ctx, existingID)
	}
	return r.ChannelAccountByID(ctx, account.ID)
}

func (r *Repository) MasterProfile(ctx context.Context, workspaceID string) (MasterProfile, error) {
	var profile MasterProfile
	err := r.db.QueryRowContext(ctx, `
		SELECT
			w.id,
			COALESCE(w.master_phone_raw, ''),
			COALESCE(w.master_phone_normalized, ''),
			EXISTS (
				SELECT 1
				FROM channel_accounts ca
				WHERE ca.workspace_id = w.id
				  AND ca.channel_kind = 'telegram_client'
				  AND COALESCE(ca.account_scope, 'workspace') = 'workspace'
				  AND ca.revoked_at IS NULL
				  AND ca.connected = TRUE
				  AND COALESCE(ca.is_enabled, TRUE) = TRUE
			)
		FROM workspaces w
		WHERE w.id = $1
	`, workspaceID).Scan(&profile.WorkspaceID, &profile.MasterPhoneRaw, &profile.MasterPhoneNormalized, &profile.TelegramEnabled)
	return profile, err
}

func (r *Repository) UpdateMasterProfile(ctx context.Context, workspaceID, rawPhone string) (MasterProfile, error) {
	normalized, err := normalizeMasterPhone(rawPhone)
	if err != nil {
		return MasterProfile{}, err
	}
	if _, err := r.db.ExecContext(ctx, `
		UPDATE workspaces
		SET master_phone_raw = $2,
			master_phone_normalized = $3
		WHERE id = $1
	`, workspaceID, strings.TrimSpace(rawPhone), normalized); err != nil {
		return MasterProfile{}, err
	}
	return r.MasterProfile(ctx, workspaceID)
}

func (r *Repository) WorkspaceByMasterPhone(ctx context.Context, normalizedPhone string) (Workspace, error) {
	var workspace Workspace
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, timezone, COALESCE(master_phone_raw, ''), COALESCE(master_phone_normalized, '')
		FROM workspaces
		WHERE master_phone_normalized = $1
	`, normalizedPhone).Scan(&workspace.ID, &workspace.Name, &workspace.Timezone, &workspace.MasterPhoneRaw, &workspace.MasterPhoneNormalized)
	return workspace, err
}

func (r *Repository) ClientBotRouteByChat(ctx context.Context, channelAccountID, externalChatID string) (ClientBotRoute, error) {
	var route ClientBotRoute
	err := r.db.QueryRowContext(ctx, `
		SELECT
			channel_account_id,
			external_chat_id,
			COALESCE(selected_workspace_id, ''),
			COALESCE(selected_master_phone_normalized, ''),
			state,
			expires_at,
			updated_at
		FROM client_bot_routes
		WHERE channel_account_id = $1
		  AND external_chat_id = $2
	`, channelAccountID, externalChatID).Scan(
		&route.ChannelAccountID,
		&route.ExternalChatID,
		&route.SelectedWorkspaceID,
		&route.SelectedMasterPhoneNormalized,
		&route.State,
		&route.ExpiresAt,
		&route.UpdatedAt,
	)
	return route, err
}

func (r *Repository) SaveClientBotRoute(ctx context.Context, route ClientBotRoute) (ClientBotRoute, error) {
	now := time.Now().UTC()
	if route.ExpiresAt.IsZero() {
		route.ExpiresAt = now.Add(30 * 24 * time.Hour)
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO client_bot_routes (
			id, channel_account_id, external_chat_id, selected_workspace_id,
			selected_master_phone_normalized, state, expires_at, updated_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8)
		ON CONFLICT (channel_account_id, external_chat_id) DO UPDATE
		SET selected_workspace_id = EXCLUDED.selected_workspace_id,
			selected_master_phone_normalized = EXCLUDED.selected_master_phone_normalized,
			state = EXCLUDED.state,
			expires_at = EXCLUDED.expires_at,
			updated_at = EXCLUDED.updated_at
	`, newID("cbr"), route.ChannelAccountID, route.ExternalChatID, route.SelectedWorkspaceID, route.SelectedMasterPhoneNormalized, route.State, route.ExpiresAt, now); err != nil {
		return ClientBotRoute{}, err
	}
	return r.ClientBotRouteByChat(ctx, route.ChannelAccountID, route.ExternalChatID)
}

func (r *Repository) ClearClientBotRoute(ctx context.Context, channelAccountID, externalChatID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM client_bot_routes
		WHERE channel_account_id = $1
		  AND external_chat_id = $2
	`, channelAccountID, externalChatID)
	return err
}

func (r *Repository) MarkTelegramUpdateProcessed(ctx context.Context, workspaceID, channelAccountID string, botKind ChannelKind, updateID int64, chatID string, messageID int64, callbackID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO telegram_update_dedup (
			id, workspace_id, channel_account_id, bot_kind, update_id, chat_id, message_id, callback_id
		)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8)
		ON CONFLICT (channel_account_id, bot_kind, update_id, chat_id, message_id, callback_id) DO NOTHING
	`, newID("tgdup"), workspaceID, channelAccountID, botKind, updateID, chatID, messageID, callbackID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *Repository) enqueueOutboundMessageTx(ctx context.Context, tx *sql.Tx, outbound OutboundMessage, payload any) (OutboundMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return OutboundMessage{}, err
	}
	now := time.Now().UTC()
	if outbound.ID == "" {
		outbound.ID = newID("out")
	}
	outbound.Payload = string(raw)
	outbound.Status = OutboundStatusQueued
	outbound.NextAttemptAt = now
	outbound.CreatedAt = now
	outbound.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO outbound_messages (
			id, workspace_id, channel, channel_kind, channel_account_id, conversation_id,
			message_id, kind, payload_json, status, retry_count, last_error, next_attempt_at,
			provider_message_id, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, $15, $16)
	`, outbound.ID, outbound.WorkspaceID, outbound.Channel, outbound.ChannelKind, outbound.ChannelAccountID, outbound.ConversationID, outbound.MessageID, outbound.Kind, outbound.Payload, outbound.Status, outbound.RetryCount, outbound.LastError, outbound.NextAttemptAt, outbound.ProviderMessageID, outbound.CreatedAt, outbound.UpdatedAt); err != nil {
		return OutboundMessage{}, err
	}
	return outbound, nil
}

func (r *Repository) EnqueueOutboundMessage(ctx context.Context, outbound OutboundMessage, payload any) (OutboundMessage, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return OutboundMessage{}, err
	}
	defer tx.Rollback()
	item, err := r.enqueueOutboundMessageTx(ctx, tx, outbound, payload)
	if err != nil {
		return OutboundMessage{}, err
	}
	if err := tx.Commit(); err != nil {
		return OutboundMessage{}, err
	}
	return item, nil
}

func (r *Repository) ClaimNextOutboundMessage(ctx context.Context) (OutboundMessage, error) {
	var item OutboundMessage
	err := r.db.QueryRowContext(ctx, `
		WITH next_item AS (
			SELECT id
			FROM outbound_messages
			WHERE status IN ('queued', 'processing')
			  AND next_attempt_at <= NOW()
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE outbound_messages om
		SET status = 'processing',
			updated_at = NOW()
		FROM next_item
		WHERE om.id = next_item.id
		RETURNING
			om.id,
			om.workspace_id,
			om.channel,
			om.channel_kind,
			om.channel_account_id,
			om.conversation_id,
			om.message_id,
			om.kind,
			om.payload_json::text,
			om.status,
			om.retry_count,
			om.last_error,
			om.next_attempt_at,
			om.provider_message_id,
			om.created_at,
			om.updated_at
	`).Scan(
		&item.ID,
		&item.WorkspaceID,
		&item.Channel,
		&item.ChannelKind,
		&item.ChannelAccountID,
		&item.ConversationID,
		&item.MessageID,
		&item.Kind,
		&item.Payload,
		&item.Status,
		&item.RetryCount,
		&item.LastError,
		&item.NextAttemptAt,
		&item.ProviderMessageID,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return OutboundMessage{}, err
	}
	return item, nil
}

func (r *Repository) MarkOutboundMessageSent(ctx context.Context, outboundID, providerMessageID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE outbound_messages
		SET status = 'sent',
			provider_message_id = COALESCE(NULLIF($2, ''), provider_message_id),
			last_error = '',
			updated_at = NOW()
		WHERE id = $1
	`, outboundID, providerMessageID)
	return err
}

func (r *Repository) ScheduleOutboundMessageRetry(ctx context.Context, outboundID, lastError string, retryCount int, nextAttemptAt time.Time) error {
	status := OutboundStatusQueued
	if retryCount >= 5 {
		status = OutboundStatusFailed
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE outbound_messages
		SET status = $2,
			retry_count = $3,
			last_error = $4,
			next_attempt_at = $5,
			updated_at = NOW()
		WHERE id = $1
	`, outboundID, status, retryCount, lastError, nextAttemptAt)
	return err
}

func (r *Repository) MarkOutboundMessageFailed(ctx context.Context, outboundID, lastError string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE outbound_messages
		SET status = 'failed',
			last_error = $2,
			updated_at = NOW()
		WHERE id = $1
	`, outboundID, lastError)
	return err
}

func (r *Repository) CreateAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	raw := []byte("{}")
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		raw = encoded
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs (id, workspace_id, user_id, action, entity_type, entity_id, payload_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, newID("audit"), workspaceID, userID, action, entityType, entityID, string(raw))
	return err
}

func (r *Repository) ActiveOperatorBindings(ctx context.Context, workspaceID string) ([]OperatorBotBinding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, workspace_id, telegram_user_id, telegram_chat_id, linked_at, is_active, last_menu_message_id
		FROM operator_bot_bindings
		WHERE workspace_id = $1 AND is_active = TRUE
		ORDER BY linked_at ASC
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []OperatorBotBinding
	for rows.Next() {
		var item OperatorBotBinding
		if err := rows.Scan(&item.UserID, &item.WorkspaceID, &item.TelegramUserID, &item.TelegramChatID, &item.LinkedAt, &item.IsActive, &item.LastMenuMessageID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) outboundTargetTx(ctx context.Context, tx *sql.Tx, workspaceID, conversationID string) (Conversation, ChannelAccount, error) {
	var conversation Conversation
	var account ChannelAccount
	err := tx.QueryRowContext(ctx, `
		SELECT
			c.id,
			c.workspace_id,
			c.customer_id,
			c.provider,
			COALESCE(c.external_chat_id, ''),
			COALESCE(c.channel_account_id, ''),
			c.status
		FROM conversations c
		WHERE c.id = $1 AND c.workspace_id = $2
		FOR UPDATE
	`, conversationID, workspaceID).Scan(
		&conversation.ID,
		&conversation.WorkspaceID,
		&conversation.CustomerID,
		&conversation.Provider,
		&conversation.ExternalChatID,
		&account.ID,
		&conversation.Status,
	)
	if err != nil {
		return Conversation{}, ChannelAccount{}, err
	}
	if account.ID == "" {
		return Conversation{}, ChannelAccount{}, errors.New("conversation has no channel account")
	}
	account, err = r.channelAccountByIDTx(ctx, tx, account.ID)
	if err != nil {
		return Conversation{}, ChannelAccount{}, err
	}
	return conversation, account, nil
}

func (r *Repository) ConversationByExternalChat(ctx context.Context, workspaceID string, provider ChannelProvider, externalChatID string) (Conversation, Customer, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Conversation{}, Customer{}, err
	}
	defer tx.Rollback()
	conversation, err := r.conversationByExternalChatTx(ctx, tx, workspaceID, provider, externalChatID)
	if err != nil {
		return Conversation{}, Customer{}, err
	}
	customer, err := r.customerTx(ctx, tx, workspaceID, conversation.CustomerID)
	if err != nil {
		return Conversation{}, Customer{}, err
	}
	return conversation, customer, nil
}

func (r *Repository) EnsureCustomerIdentity(ctx context.Context, workspaceID string, provider ChannelProvider, externalChatID string, profile InboundProfile) (Customer, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Customer{}, err
	}
	defer tx.Rollback()
	customer, err := r.upsertCustomerIdentityTx(ctx, tx, workspaceID, provider, externalChatID, profile)
	if err != nil {
		return Customer{}, err
	}
	if err := tx.Commit(); err != nil {
		return Customer{}, err
	}
	return customer, nil
}
