package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgapi "github.com/vital/rendycrm-app/internal/telegram"
)

// outboundDispatchCooldown must exceed outboundProcessingLease (2 min) so that
// the Redis guard is still held when the DB lease expires, preventing a second
// dispatcher from claiming and re-sending an in-flight outbound message.
const outboundDispatchCooldown = 3 * time.Minute

func telegramInlineKeyboardRows(buttons []TelegramInlineButton) [][]tgapi.InlineKeyboardButton {
	rows := make([][]tgapi.InlineKeyboardButton, 0, len(buttons))
	for _, button := range buttons {
		if strings.TrimSpace(button.Text) == "" {
			continue
		}
		if url := strings.TrimSpace(button.URL); url != "" {
			rows = append(rows, []tgapi.InlineKeyboardButton{{Text: button.Text, URL: url}})
			continue
		}
		if callbackData := strings.TrimSpace(button.CallbackData); callbackData != "" {
			rows = append(rows, []tgapi.InlineKeyboardButton{{Text: button.Text, CallbackData: callbackData}})
		}
	}
	return rows
}

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
				select {
				case <-s.outboundWake:
				case <-time.After(500 * time.Millisecond):
				case <-ctx.Done():
					return
				}
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
		rows := telegramInlineKeyboardRows(payload.Buttons)
		res, err := s.runtime.telegram.SendInline(ctx, token, payload.ChatID, payload.Text, rows)
		if err != nil {
			return s.retryOutboundMessage(ctx, item, err)
		}
		providerMessageID = fmt.Sprintf("%d", res.MessageID)
	case OutboundKindTelegramEditInline:
		rows := telegramInlineKeyboardRows(payload.Buttons)
		request := tgapi.EditMessageTextRequest{
			ChatID:      payload.ChatID,
			MessageID:   payload.MessageID,
			Text:        payload.Text,
			ReplyMarkup: tgapi.InlineKeyboardMarkup{InlineKeyboard: rows},
		}
		res, err := s.runtime.telegram.EditInline(ctx, token, request)
		if err != nil {
			if isTelegramEditNoopError(err) {
				providerMessageID = strconv.FormatInt(payload.MessageID, 10)
				break
			}
			if shouldConfirmTelegramEdit(err) {
				confirmedMessageID, confirmErr := s.confirmTelegramEdit(ctx, token, request)
				if confirmErr == nil {
					providerMessageID = confirmedMessageID
					break
				}
				err = confirmErr
			}
			if shouldAssumeTelegramEditApplied(item, payload, err) {
				log.Printf("telegram edit assumed applied id=%s message_id=%d chat_id=%s error=%v", item.ID, payload.MessageID, payload.ChatID, err)
				providerMessageID = strconv.FormatInt(payload.MessageID, 10)
				break
			}
			if payload.EditFallbackToSend && shouldFallbackTelegramEditToSend(err) {
				sendRes, sendErr := s.runtime.telegram.SendInline(ctx, token, payload.ChatID, payload.Text, rows)
				if sendErr != nil {
					return s.retryOutboundMessage(ctx, item, sendErr)
				}
				providerMessageID = fmt.Sprintf("%d", sendRes.MessageID)
				break
			}
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

	if err := s.runtime.repository.MarkOutboundMessageSent(ctx, item.ID, providerMessageID); err != nil {
		return err
	}
	s.rememberTelegramOperatorMenuMessage(ctx, item, payload, providerMessageID)
	return nil
}

func isTelegramEditNoopError(err error) bool {
	var apiErr *tgapi.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != 400 {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(apiErr.Description)), "message is not modified")
}

func shouldConfirmTelegramEdit(err error) bool {
	var apiErr *tgapi.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == 400
}

func (s *Server) confirmTelegramEdit(ctx context.Context, token string, request tgapi.EditMessageTextRequest) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(350 * time.Millisecond):
	}
	res, err := s.runtime.telegram.EditInline(ctx, token, request)
	if err == nil {
		return fmt.Sprintf("%d", res.MessageID), nil
	}
	if isTelegramEditNoopError(err) {
		return strconv.FormatInt(request.MessageID, 10), nil
	}
	return "", err
}

func shouldAssumeTelegramEditApplied(item OutboundMessage, payload TelegramOutboundPayload, err error) bool {
	if item.ChannelKind != ChannelKindTelegramOperator {
		return false
	}
	if payload.MessageID <= 0 || strings.TrimSpace(payload.ChatID) == "" || len(payload.Buttons) == 0 {
		return false
	}
	var apiErr *tgapi.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == 400
}

func shouldFallbackTelegramEditToSend(err error) bool {
	var apiErr *tgapi.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == 400
}

func (s *Server) rememberTelegramOperatorMenuMessage(ctx context.Context, item OutboundMessage, payload TelegramOutboundPayload, providerMessageID string) {
	if item.ChannelKind != ChannelKindTelegramOperator || strings.TrimSpace(payload.ChatID) == "" || len(payload.Buttons) == 0 {
		return
	}
	messageID, err := strconv.ParseInt(strings.TrimSpace(providerMessageID), 10, 64)
	if err != nil || messageID <= 0 {
		return
	}
	s.rememberTelegramOperatorRuntimeMessageID(ctx, item.ChannelAccountID, payload.ChatID, messageID)
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
