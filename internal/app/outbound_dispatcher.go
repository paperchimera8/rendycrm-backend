package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	tgapi "github.com/vital/rendycrm-app/internal/telegram"
)

// outboundDispatchCooldown must exceed outboundProcessingLease (2 min) so that
// the Redis guard is still held when the DB lease expires, preventing a second
// dispatcher from claiming and re-sending an in-flight outbound message.
const outboundDispatchCooldown = 3 * time.Minute

func (s *Server) processOutboundMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		item, err := s.runtime.repository.ClaimNextOutboundMessage(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				time.Sleep(500 * time.Millisecond)
				continue
			}
			log.Printf("outbound claim error: %v", err)
			time.Sleep(time.Second)
			continue
		}
		freshDispatch, err := s.claimOutboundDispatch(ctx, item.ID)
		if err != nil {
			log.Printf("outbound dispatch lock error id=%s: %v", item.ID, err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if !freshDispatch {
			log.Printf("outbound dispatch skipped duplicate id=%s kind=%s", item.ID, item.Kind)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err := s.dispatchOutboundMessage(ctx, item); err != nil {
			log.Printf("outbound dispatch failed id=%s kind=%s: %v", item.ID, item.Kind, err)
		}
	}
}

func (s *Server) claimOutboundDispatch(ctx context.Context, outboundID string) (bool, error) {
	if strings.TrimSpace(outboundID) == "" {
		return false, errors.New("outbound id is empty")
	}
	return s.runtime.redis.SetNX(ctx, "telegram:outbound-dispatch:"+outboundID, "1", outboundDispatchCooldown).Result()
}

func (s *Server) dispatchOutboundMessage(ctx context.Context, item OutboundMessage) error {
	account, err := s.runtime.repository.ChannelAccountByID(ctx, item.ChannelAccountID)
	if err != nil {
		_ = s.runtime.repository.MarkOutboundMessageFailed(ctx, item.ID, err.Error())
		return err
	}
	token, err := decryptString(s.cfg.EncryptionSecret, account.EncryptedToken)
	if err != nil {
		if strings.TrimSpace(account.EncryptedToken) == "" {
			_ = s.runtime.repository.MarkOutboundMessageFailed(ctx, item.ID, "telegram token not configured")
			return errors.New("telegram token not configured")
		}
		_ = s.runtime.repository.MarkOutboundMessageFailed(ctx, item.ID, err.Error())
		return err
	}

	var payload TelegramOutboundPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		_ = s.runtime.repository.MarkOutboundMessageFailed(ctx, item.ID, err.Error())
		return err
	}

	var providerMessageID string
	switch item.Kind {
	case OutboundKindTelegramSendText:
		res, err := s.runtime.telegram.SendText(ctx, token, tgapi.SendMessageRequest{
			ChatID: payload.ChatID,
			Text:   payload.Text,
		})
		if err != nil {
			return s.retryOutboundMessage(ctx, item, err)
		}
		providerMessageID = fmt.Sprintf("%d", res.MessageID)
	case OutboundKindTelegramSendInline:
		rows := make([][]tgapi.InlineKeyboardButton, 0, len(payload.Buttons))
		for _, button := range payload.Buttons {
			rows = append(rows, []tgapi.InlineKeyboardButton{{Text: button.Text, CallbackData: button.CallbackData}})
		}
		res, err := s.runtime.telegram.SendInline(ctx, token, payload.ChatID, payload.Text, rows)
		if err != nil {
			return s.retryOutboundMessage(ctx, item, err)
		}
		providerMessageID = fmt.Sprintf("%d", res.MessageID)
	case OutboundKindTelegramEditInline:
		rows := make([][]tgapi.InlineKeyboardButton, 0, len(payload.Buttons))
		for _, button := range payload.Buttons {
			rows = append(rows, []tgapi.InlineKeyboardButton{{Text: button.Text, CallbackData: button.CallbackData}})
		}
		res, err := s.runtime.telegram.EditInline(ctx, token, tgapi.EditMessageTextRequest{
			ChatID:      payload.ChatID,
			MessageID:   payload.MessageID,
			Text:        payload.Text,
			ReplyMarkup: tgapi.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		if err != nil {
			return s.retryOutboundMessage(ctx, item, err)
		}
		providerMessageID = fmt.Sprintf("%d", res.MessageID)
	case OutboundKindTelegramAnswerCBQ:
		if err := s.runtime.telegram.AnswerCallback(ctx, token, tgapi.AnswerCallbackQueryRequest{
			CallbackQueryID: payload.CallbackID,
			Text:            payload.CallbackText,
			ShowAlert:       payload.ShowAlert,
		}); err != nil {
			return s.retryOutboundMessage(ctx, item, err)
		}
	default:
		_ = s.runtime.repository.MarkOutboundMessageFailed(ctx, item.ID, "unsupported outbound kind")
		return fmt.Errorf("unsupported outbound kind %s", item.Kind)
	}

	return s.runtime.repository.MarkOutboundMessageSent(ctx, item.ID, providerMessageID)
}

func (s *Server) retryOutboundMessage(ctx context.Context, item OutboundMessage, err error) error {
	if !tgapi.IsRetriableError(err) {
		if markErr := s.runtime.repository.MarkOutboundMessageFailed(ctx, item.ID, err.Error()); markErr != nil {
			return markErr
		}
		return err
	}
	retryCount := item.RetryCount + 1
	nextAttemptAt := time.Now().UTC().Add(time.Duration(1<<min(retryCount, 5)) * time.Second)
	if scheduleErr := s.runtime.repository.ScheduleOutboundMessageRetry(ctx, item.ID, err.Error(), retryCount, nextAttemptAt); scheduleErr != nil {
		return scheduleErr
	}
	return err
}
