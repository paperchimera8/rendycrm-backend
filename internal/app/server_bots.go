package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
	telegram "github.com/vital/rendycrm-app/internal/telegram"
	"github.com/vital/rendycrm-app/internal/usecase"
)

type telegramUpdate struct {
	Message *struct {
		MessageID int64  `json:"message_id"`
		Date      int64  `json:"date"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"from"`
	} `json:"message"`
	CallbackQuery *struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"from"`
		Message struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

func (s *Server) handleOperatorBotSettings(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/settings/operator-bot":
		settings, err := s.runtime.repository.OperatorBotSettings(r.Context(), auth.Workspace.ID, auth.User.ID, s.cfg.OperatorBotUsername, s.cfg.PublicBaseURL)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "operator bot settings query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, settings)
	case r.Method == http.MethodPost && r.URL.Path == "/settings/operator-bot/link-code":
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		code, err := s.runtime.services.OperatorLink.CreateLinkCode(r.Context(), actor, auth.Workspace.ID, auth.User.ID, s.cfg.OperatorBotUsername)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, "failed to create operator link code")
			return
		}
		s.writeJSON(w, http.StatusCreated, OperatorBotLinkCode{
			ID:          code.ID,
			UserID:      code.UserID,
			WorkspaceID: code.WorkspaceID,
			Code:        code.Code,
			ExpiresAt:   code.ExpiresAt,
			DeepLink:    code.DeepLink,
		})
	case r.Method == http.MethodPost && r.URL.Path == "/settings/operator-bot/unlink":
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		if err := s.runtime.services.OperatorLink.UnlinkTelegram(r.Context(), actor, auth.Workspace.ID, auth.User.ID); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, "failed to unlink operator bot")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case r.Method == http.MethodPut && r.URL.Path == "/settings/operator-bot":
		var payload struct {
			BotUsername   string `json:"botUsername"`
			BotToken      string `json:"botToken"`
			WebhookSecret string `json:"webhookSecret"`
			Enabled       bool   `json:"enabled"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		encryptedToken := ""
		if strings.TrimSpace(payload.BotToken) != "" {
			encrypted, err := encryptString(s.cfg.EncryptionSecret, payload.BotToken)
			if err != nil {
				s.writeError(w, http.StatusInternalServerError, "failed to encrypt bot token")
				return
			}
			encryptedToken = encrypted
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		accountResult, err := s.runtime.services.Channels.SaveChannelSettings(r.Context(), actor, usecase.ChannelAccountInput{
			WorkspaceID:     auth.Workspace.ID,
			Provider:        string(ChannelTelegram),
			ChannelKind:     string(ChannelKindTelegramOperator),
			AccountScope:    string(ChannelAccountScopeWorkspace),
			Name:            "Telegram operator bot",
			Connected:       payload.Enabled,
			IsEnabled:       payload.Enabled,
			BotUsername:     strings.TrimSpace(payload.BotUsername),
			WebhookSecret:   strings.TrimSpace(payload.WebhookSecret),
			EncryptedToken:  encryptedToken,
			TokenConfigured: encryptedToken != "",
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, "failed to save operator bot settings")
			return
		}
		account, err := s.runtime.repository.ChannelAccountByID(r.Context(), accountResult.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "operator bot settings query failed")
			return
		}
		settings, err := s.runtime.repository.OperatorBotSettings(r.Context(), auth.Workspace.ID, auth.User.ID, account.BotUsername, s.cfg.PublicBaseURL)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "operator bot settings query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, settings)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleTelegramOperatorWebhook(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))
	if secret == "" {
		s.writeError(w, http.StatusUnauthorized, "missing telegram webhook secret")
		return
	}
	account, err := s.runtime.repository.ChannelAccountByWebhookSecret(r.Context(), ChannelKindTelegramOperator, secret)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "invalid telegram webhook secret")
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}
	update, err := telegram.ParseUpdate(body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}
	if err := s.handleTelegramOperatorUpdate(r.Context(), account, update); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to process operator update")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func parseOperatorTelegramUpdate(update telegramUpdate) (chatID string, userID string, text string, err error) {
	switch {
	case update.Message != nil:
		chatID = strconv.FormatInt(update.Message.Chat.ID, 10)
		userID = strconv.FormatInt(update.Message.From.ID, 10)
		text = strings.TrimSpace(update.Message.Text)
	case update.CallbackQuery != nil:
		chatID = strconv.FormatInt(update.CallbackQuery.Message.Chat.ID, 10)
		userID = strconv.FormatInt(update.CallbackQuery.From.ID, 10)
		text = strings.TrimSpace(update.CallbackQuery.Data)
	default:
		return "", "", "", errors.New("empty telegram update")
	}
	if text == "" {
		return "", "", "", errors.New("telegram update has no text")
	}
	return chatID, userID, text, nil
}

func (s *Server) handleOperatorSession(ctx context.Context, binding OperatorBotBinding, session BotSession, text string) (bool, []BotOutboundMessage, error) {
	actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: binding.WorkspaceID, UserID: binding.UserID}
	switch session.State {
	case "awaiting_operator_reply":
		var payload struct {
			ConversationID string `json:"conversationId"`
		}
		if err := json.Unmarshal([]byte(session.Payload), &payload); err != nil {
			return false, nil, err
		}
		if _, err := s.runtime.services.Dialogs.ReplyToDialog(ctx, actor, payload.ConversationID, text); err != nil {
			return false, nil, err
		}
		if err := s.runtime.services.BotSessions.ClearSession(ctx, actor, binding.WorkspaceID, string(BotSessionScopeOperator), string(BotSessionActorUser), binding.UserID); err != nil {
			return false, nil, err
		}
		return true, []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Ответ отправлен клиенту."}}, nil
	case "awaiting_price":
		var payload struct {
			ConversationID string `json:"conversationId"`
			CustomerID     string `json:"customerId"`
			DailySlotID    string `json:"dailySlotId"`
		}
		if err := json.Unmarshal([]byte(session.Payload), &payload); err != nil {
			return false, nil, err
		}
		amount, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || amount <= 0 {
			return true, []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Введите цену числом, например 4500."}}, nil
		}
		booking, err := s.runtime.services.Bookings.CreateBooking(ctx, actor, usecase.CreateBookingInput{
			WorkspaceID:    binding.WorkspaceID,
			CustomerID:     payload.CustomerID,
			DailySlotID:    payload.DailySlotID,
			Amount:         amount,
			Status:         "confirmed",
			Notes:          "Подтверждено через Telegram operator bot",
			ConversationID: payload.ConversationID,
		})
		if err != nil {
			return false, nil, err
		}
		if err := s.runtime.services.BotSessions.ClearSession(ctx, actor, binding.WorkspaceID, string(BotSessionScopeOperator), string(BotSessionActorUser), binding.UserID); err != nil {
			return false, nil, err
		}
		return true, []BotOutboundMessage{{
			ChatID: binding.TelegramChatID,
			Text:   fmt.Sprintf("Запись подтверждена: %s за %d ₽.", booking.StartsAt.In(time.Local).Format("02.01 15:04"), amount),
		}}, nil
	}
	return false, nil, nil
}

func (s *Server) handleOperatorCommand(ctx context.Context, binding OperatorBotBinding, text string) ([]BotOutboundMessage, error) {
	command := strings.TrimSpace(text)
	actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: binding.WorkspaceID, UserID: binding.UserID}
	switch {
	case command == "/dashboard" || command == "📊 Дашборд":
		dashboard, err := s.runtime.repository.Dashboard(ctx, binding.WorkspaceID)
		if err != nil {
			return nil, err
		}
		bookings, err := s.runtime.repository.Bookings(ctx, binding.WorkspaceID, "all")
		if err != nil {
			return nil, err
		}
		nextSlot := "нет"
		sort.Slice(bookings, func(i, j int) bool { return bookings[i].StartsAt.Before(bookings[j].StartsAt) })
		for _, booking := range bookings {
			if booking.Status == BookingCancelled {
				continue
			}
			if booking.StartsAt.After(time.Now().UTC()) {
				nextSlot = booking.StartsAt.In(time.Local).Format("Mon 02.01 15:04")
				break
			}
		}
		text := fmt.Sprintf(
			"📊 Rendy CRM\n\nЗаписей сегодня: %d\nНовых обращений: %d\nДоход: %d ₽\nСвободных окон: %d\nБлижайший слот: %s",
			dashboard.TodayBookings,
			dashboard.NewMessages,
			0,
			0,
			nextSlot,
		)
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: text, Buttons: []string{"/dialogs", "/slots", "/settings"}}}, nil
	case command == "/dialogs" || command == "💬 Диалоги" || command == "🔥 Новые":
		conversations, err := s.runtime.repository.Conversations(ctx, binding.WorkspaceID)
		if err != nil {
			return nil, err
		}
		lines := []string{"💬 Обращения"}
		buttons := make([]string, 0, min(5, len(conversations)))
		count := 0
		for _, conversation := range conversations {
			if conversation.Status == ConversationClosed {
				continue
			}
			if conversation.UnreadCount == 0 && conversation.Status == ConversationBooked {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s [%s] — %s", conversation.Title, conversation.Provider, conversation.LastMessageText))
			buttons = append(buttons, "dlg:open:"+conversation.ID)
			count++
			if count >= 5 {
				break
			}
		}
		if count == 0 {
			lines = append(lines, "Новых обращений нет.")
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: strings.Join(lines, "\n"), Buttons: buttons}}, nil
	case command == "/slots" || command == "🕒 Слоты":
		days, err := s.runtime.repository.WeekSlots(ctx, binding.WorkspaceID, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		lines := []string{"🕒 Слоты на неделю"}
		for _, day := range days {
			lines = append(lines, fmt.Sprintf("%s — %d", day.Label, len(day.Slots)))
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: strings.Join(lines, "\n")}}, nil
	case command == "/settings" || command == "⚙️ Настройки":
		settings, err := s.runtime.repository.OperatorBotSettings(ctx, binding.WorkspaceID, binding.UserID, s.cfg.OperatorBotUsername, s.cfg.PublicBaseURL)
		if err != nil {
			return nil, err
		}
		botConfig, _, err := s.runtime.repository.BotConfig(ctx, binding.WorkspaceID)
		if err != nil {
			return nil, err
		}
		chatLabel := "не привязан"
		if settings.Binding != nil {
			chatLabel = settings.Binding.TelegramChatID
		}
		text := fmt.Sprintf(
			"⚙️ Настройки\n\nАвтоответ: %t\nHandoff: %t\nTelegram chat: %s\nWebhook: %s",
			botConfig.AutoReply,
			botConfig.HandoffEnabled,
			chatLabel,
			settings.OperatorWebhookURL,
		)
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: text}}, nil
	case command == "/faq" || command == "❓ FAQ":
		_, faqItems, err := s.runtime.repository.BotConfig(ctx, binding.WorkspaceID)
		if err != nil {
			return nil, err
		}
		lines := []string{"❓ FAQ"}
		for _, item := range faqItems {
			lines = append(lines, "• "+item.Question)
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: strings.Join(lines, "\n")}}, nil
	case strings.HasPrefix(command, "dialog:") || strings.HasPrefix(command, "/dialog "):
		id := strings.TrimPrefix(command, "dialog:")
		id = strings.TrimSpace(strings.TrimPrefix(id, "/dialog"))
		conversation, messages, customer, err := s.runtime.repository.ConversationDetail(ctx, binding.WorkspaceID, id)
		if err != nil {
			return nil, err
		}
		lastMessage := conversation.LastMessageText
		if len(messages) > 0 {
			lastMessage = messages[len(messages)-1].Text
		}
		text := fmt.Sprintf(
			"👤 %s\nКанал: %s\nТелефон: %s\nСтатус: %s\n\nПоследнее сообщение:\n%s",
			customer.Name,
			conversation.Provider,
			customer.Phone,
			conversation.Status,
			lastMessage,
		)
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: text, Buttons: []string{"dlg:reply:" + conversation.ID, "slot:list:" + conversation.ID, "dlg:close:" + conversation.ID}}}, nil
	case strings.HasPrefix(command, "reply:"):
		conversationID := strings.TrimPrefix(command, "reply:")
		if _, err := s.runtime.services.BotSessions.StartSession(ctx, actor, usecase.BotSessionInput{
			WorkspaceID: binding.WorkspaceID,
			BotKind:     string(ChannelKindTelegramOperator),
			Scope:       string(BotSessionScopeOperator),
			ActorType:   string(BotSessionActorUser),
			ActorID:     binding.UserID,
			State:       "awaiting_operator_reply",
			ExpiresAt:   time.Now().UTC().Add(15 * time.Minute),
		}, map[string]string{"conversationId": conversationID}); err != nil {
			return nil, err
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Введите ответ клиенту одним сообщением."}}, nil
	case strings.HasPrefix(command, "slots:"):
		conversationID := strings.TrimPrefix(command, "slots:")
		conversation, _, _, err := s.runtime.repository.ConversationDetail(ctx, binding.WorkspaceID, conversationID)
		if err != nil {
			return nil, err
		}
		slots, err := s.runtime.repository.AvailableDaySlots(ctx, binding.WorkspaceID, time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 14))
		if err != nil {
			return nil, err
		}
		response, _ := formatSlotsForBot(slots)
		buttons := make([]string, 0, min(3, len(slots)))
		for i, slot := range slots {
			if i >= 3 {
				break
			}
			buttons = append(buttons, "pickslot:"+conversation.ID+":"+slot.ID)
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: response, Buttons: buttons}}, nil
	case strings.HasPrefix(command, "pickslot:"):
		parts := strings.Split(command, ":")
		if len(parts) != 3 {
			return nil, errors.New("invalid pickslot payload")
		}
		conversation, _, customer, err := s.runtime.repository.ConversationDetail(ctx, binding.WorkspaceID, parts[1])
		if err != nil {
			return nil, err
		}
		if _, err := s.runtime.services.BotSessions.StartSession(ctx, actor, usecase.BotSessionInput{
			WorkspaceID: binding.WorkspaceID,
			BotKind:     string(ChannelKindTelegramOperator),
			Scope:       string(BotSessionScopeOperator),
			ActorType:   string(BotSessionActorUser),
			ActorID:     binding.UserID,
			State:       "awaiting_price",
			ExpiresAt:   time.Now().UTC().Add(15 * time.Minute),
		}, map[string]string{"conversationId": conversation.ID, "customerId": customer.ID, "dailySlotId": parts[2]}); err != nil {
			return nil, err
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Введите цену для подтверждения записи, например 4500."}}, nil
	case strings.HasPrefix(command, "take:"):
		conversationID := strings.TrimPrefix(command, "take:")
		if err := s.runtime.services.Dialogs.TakeDialogByHuman(ctx, actor, conversationID); err != nil {
			return nil, err
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Диалог взят оператором."}}, nil
	case strings.HasPrefix(command, "auto:"):
		conversationID := strings.TrimPrefix(command, "auto:")
		if err := s.runtime.services.Dialogs.ReturnDialogToAuto(ctx, actor, conversationID); err != nil {
			return nil, err
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Диалог возвращён в автоответ."}}, nil
	case strings.HasPrefix(command, "close:"):
		conversationID := strings.TrimPrefix(command, "close:")
		if err := s.runtime.services.Dialogs.CloseDialog(ctx, actor, conversationID); err != nil {
			return nil, err
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Диалог закрыт."}}, nil
	case command == "/auto_on":
		if err := s.runtime.services.BotSettings.ToggleAutoReply(ctx, actor, binding.WorkspaceID, true); err != nil {
			return nil, err
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Автоответ включён."}}, nil
	case command == "/auto_off":
		if err := s.runtime.services.BotSettings.ToggleAutoReply(ctx, actor, binding.WorkspaceID, false); err != nil {
			return nil, err
		}
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Автоответ выключен."}}, nil
	default:
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: "Доступные команды: /dashboard, /dialogs, /slots, /settings, /faq."}}, nil
	}
}

func (s *Server) handleTelegramClientWebhook(w http.ResponseWriter, r *http.Request, channelAccountID, secret string) {
	account, err := s.runtime.repository.ChannelAccountByID(r.Context(), channelAccountID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "channel account not found")
		return
	}
	if account.Provider != ChannelTelegram || account.ChannelKind != ChannelKindTelegramClient {
		s.writeError(w, http.StatusBadRequest, "channel account is not telegram")
		return
	}
	if account.WebhookSecret != secret {
		s.writeError(w, http.StatusUnauthorized, "invalid webhook secret")
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}
	update, err := telegram.ParseUpdate(body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}
	if err := s.handleTelegramClientUpdate(r.Context(), account, update); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to receive telegram inbound")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWhatsAppWebhook(w http.ResponseWriter, r *http.Request, channelAccountID, secret string) {
	account, err := s.runtime.repository.ChannelAccountByID(r.Context(), channelAccountID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "channel account not found")
		return
	}
	if account.Provider != ChannelWhatsApp {
		s.writeError(w, http.StatusBadRequest, "channel account is not whatsapp")
		return
	}
	if !verifyTwilioSignature(r, secret) {
		s.writeError(w, http.StatusUnauthorized, "invalid twilio signature")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid form payload")
		return
	}
	body := strings.TrimSpace(r.Form.Get("Body"))
	if body == "" {
		s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	result, err := s.runtime.services.Inbox.ReceiveInboundMessage(r.Context(), usecase.InboundInput{
		Provider:          string(ChannelWhatsApp),
		ChannelAccountID:  channelAccountID,
		ExternalChatID:    r.Form.Get("From"),
		ExternalMessageID: r.Form.Get("MessageSid"),
		Text:              body,
		Timestamp:         time.Now().UTC(),
		Profile: usecase.InboundProfile{
			Name:     r.Form.Get("ProfileName"),
			Phone:    r.Form.Get("From"),
			Username: r.Form.Get("WaId"),
		},
	})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to receive whatsapp inbound")
		return
	}
	_ = s.publishEvent(r.Context(), SSEEvent{Type: EventMessageNew, Data: Message{ID: result.MessageID, ConversationID: result.ConversationID}})
	_ = s.publishDashboard(r.Context(), result.WorkspaceID)
	if conversation, _, customer, detailErr := s.runtime.repository.ConversationDetail(r.Context(), result.WorkspaceID, result.ConversationID); detailErr == nil {
		s.notifyOperatorsAboutConversation(r.Context(), conversation, customer)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func verifyTwilioSignature(r *http.Request, secret string) bool {
	if strings.TrimSpace(secret) == "" {
		return false
	}
	signature := strings.TrimSpace(r.Header.Get("X-Twilio-Signature"))
	if signature == "" {
		return false
	}
	if err := r.ParseForm(); err != nil {
		return false
	}
	values := make(url.Values)
	for key, value := range r.PostForm {
		values[key] = append([]string(nil), value...)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	base := strings.TrimRight(r.URL.String(), "?")
	var builder strings.Builder
	builder.WriteString(base)
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(values.Get(key))
	}
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = io.WriteString(mac, builder.String())
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
