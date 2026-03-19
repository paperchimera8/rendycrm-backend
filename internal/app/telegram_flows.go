package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
	tgapi "github.com/vital/rendycrm-app/internal/telegram"
	"github.com/vital/rendycrm-app/internal/usecase"
)

func (s *Server) enqueueTelegramOutbound(ctx context.Context, account ChannelAccount, kind OutboundKind, conversationID, messageID string, payload TelegramOutboundPayload) error {
	if account.ID == "" {
		return errors.New("telegram channel account is empty")
	}
	log.Printf(
		"telegram outbound bot=%s kind=%s account_id=%s conversation_id=%s message_id=%s chat_id=%s buttons=%d text=%q",
		account.ChannelKind,
		kind,
		account.ID,
		conversationID,
		messageID,
		payload.ChatID,
		len(payload.Buttons),
		telegramLogValue(payload.Text),
	)
	_, err := s.runtime.repository.EnqueueOutboundMessage(ctx, OutboundMessage{
		WorkspaceID:      account.WorkspaceID,
		Channel:          account.Provider,
		ChannelKind:      account.ChannelKind,
		ChannelAccountID: account.ID,
		ConversationID:   conversationID,
		MessageID:        messageID,
		Kind:             kind,
	}, payload)
	return err
}

func telegramButtonsFromCommands(commands []string) []TelegramInlineButton {
	buttons := make([]TelegramInlineButton, 0, len(commands))
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		buttons = append(buttons, TelegramInlineButton{
			Text:         telegramButtonLabel(command),
			CallbackData: command,
		})
	}
	return buttons
}

func telegramButtonLabel(command string) string {
	switch {
	case command == "/dashboard":
		return "Дашборд"
	case command == "/dialogs":
		return "Диалоги"
	case command == "/slots":
		return "Слоты"
	case command == "/settings":
		return "Настройки"
	case command == "/faq":
		return "FAQ"
	case command == "client:book":
		return "Записаться"
	case command == "client:slots":
		return "Свободные окна"
	case command == "client:prices":
		return "Цены"
	case command == "client:address":
		return "Адрес"
	case command == "client:human":
		return "Связаться с человеком"
	case command == "client:change_master":
		return "Сменить мастера"
	case command == "client:enter_master_phone":
		return "Ввести номер мастера"
	case strings.HasPrefix(command, "dlg:open:"), strings.HasPrefix(command, "dialog:"):
		return "Открыть диалог"
	case strings.HasPrefix(command, "dlg:reply:"), strings.HasPrefix(command, "reply:"):
		return "Ответить"
	case strings.HasPrefix(command, "slot:list:"), strings.HasPrefix(command, "slots:"):
		return "Предложить слот"
	case strings.HasPrefix(command, "dlg:close:"), strings.HasPrefix(command, "close:"):
		return "Закрыть"
	case strings.HasPrefix(command, "slot:pick:"), strings.HasPrefix(command, "pickslot:"), strings.HasPrefix(command, "slot:select:"):
		return "Выбрать"
	case strings.HasPrefix(command, "booking:confirm:"), strings.HasPrefix(command, "book:confirm:"):
		return "Подтвердить"
	case command == "booking:cancel":
		return "Отмена"
	case command == "set:auto:on":
		return "Автоответ: вкл"
	case command == "set:auto:off":
		return "Автоответ: выкл"
	default:
		return command
	}
}

const telegramCallbackActionCooldown = 10 * time.Second
const telegramInboundDeliveryCooldown = 2 * time.Minute

func telegramCallbackActionKey(accountID string, botKind ChannelKind, chatID string, messageID int64, data string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(data)))
	return fmt.Sprintf(
		"tg:cbq:%s:%s:%s:%d:%s",
		strings.TrimSpace(accountID),
		strings.TrimSpace(string(botKind)),
		strings.TrimSpace(chatID),
		messageID,
		hex.EncodeToString(hash[:8]),
	)
}

func telegramInboundDeliveryKey(accountID string, botKind ChannelKind, chatID string, messageID int64, callbackID string) string {
	if trimmed := strings.TrimSpace(callbackID); trimmed != "" {
		hash := sha256.Sum256([]byte(trimmed))
		return fmt.Sprintf(
			"tg:upd:%s:%s:%s:cbq:%s",
			strings.TrimSpace(accountID),
			strings.TrimSpace(string(botKind)),
			strings.TrimSpace(chatID),
			hex.EncodeToString(hash[:8]),
		)
	}
	return fmt.Sprintf(
		"tg:upd:%s:%s:%s:msg:%d",
		strings.TrimSpace(accountID),
		strings.TrimSpace(string(botKind)),
		strings.TrimSpace(chatID),
		messageID,
	)
}

func telegramLogValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}

func telegramUpdateDebugValue(update tgapi.Update) string {
	switch {
	case update.CallbackQuery != nil:
		return telegramLogValue(update.CallbackQuery.Data)
	case update.Message != nil:
		return telegramLogValue(update.Message.Text)
	default:
		return ""
	}
}

func (s *Server) claimTelegramCallbackAction(ctx context.Context, accountID string, botKind ChannelKind, chatID string, messageID int64, data string) (bool, error) {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(chatID) == "" || messageID == 0 || strings.TrimSpace(data) == "" {
		return true, nil
	}
	if s == nil || s.runtime == nil || s.runtime.redis == nil {
		return true, nil
	}
	key := telegramCallbackActionKey(accountID, botKind, chatID, messageID, data)
	return s.runtime.redis.SetNX(ctx, key, "1", telegramCallbackActionCooldown).Result()
}

func (s *Server) claimTelegramInboundDelivery(ctx context.Context, accountID string, botKind ChannelKind, chatID string, messageID int64, callbackID string) (bool, error) {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(chatID) == "" {
		return true, nil
	}
	if messageID == 0 && strings.TrimSpace(callbackID) == "" {
		return true, nil
	}
	if s == nil || s.runtime == nil || s.runtime.redis == nil {
		return true, nil
	}
	key := telegramInboundDeliveryKey(accountID, botKind, chatID, messageID, callbackID)
	return s.runtime.redis.SetNX(ctx, key, "1", telegramInboundDeliveryCooldown).Result()
}

func (s *Server) clientBotButtons(account ChannelAccount, chatID, workspaceID string, includeChangeMaster bool) []TelegramInlineButton {
	buttons := make([]TelegramInlineButton, 0, 5)
	if link, err := s.clientCalendarURL(account, chatID, workspaceID); err == nil && strings.TrimSpace(link) != "" {
		buttons = append(buttons, TelegramInlineButton{Text: "Записаться", URL: link})
	} else {
		buttons = append(buttons, TelegramInlineButton{Text: "Записаться", CallbackData: "client:book"})
	}
	buttons = append(buttons,
		TelegramInlineButton{Text: "Цены", CallbackData: "client:prices"},
		TelegramInlineButton{Text: "Адрес", CallbackData: "client:address"},
		TelegramInlineButton{Text: "Связаться с человеком", CallbackData: "client:human"},
	)
	if includeChangeMaster {
		buttons = append(buttons, TelegramInlineButton{Text: "Сменить мастера", CallbackData: "client:change_master"})
	}
	return buttons
}

func (s *Server) sendTelegramCalendarPrompt(ctx context.Context, account ChannelAccount, chatID, workspaceID string) error {
	link, err := s.clientCalendarURL(account, chatID, workspaceID)
	if err != nil || strings.TrimSpace(link) == "" {
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Не удалось открыть календарь записи. Напишите сообщение в этот чат, и мастер свяжется с вами.",
		})
	}
	return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
		ChatID: chatID,
		Text:   "Откройте календарь и выберите свободный слот. Данные обновляются автоматически.",
		Buttons: []TelegramInlineButton{
			{Text: "Открыть календарь", URL: link},
		},
	})
}

func (s *Server) promptTelegramMasterPhone(ctx context.Context, account ChannelAccount, chatID string) error {
	_, _ = s.runtime.services.BotSessions.StoreClientRoute(ctx, domain.SystemActor(), usecase.ClientBotRouteInput{
		ChannelAccountID: account.ID,
		ExternalChatID:   chatID,
		State:            "awaiting_master_phone",
		ExpiresAt:        time.Now().UTC().Add(30 * 24 * time.Hour),
	})
	return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
		ChatID: chatID,
		Text:   "Введите номер мастера, к которому хотите записаться.",
		Buttons: []TelegramInlineButton{
			{Text: "Ввести номер мастера", CallbackData: "client:enter_master_phone"},
		},
	})
}

func (s *Server) selectedClientRoute(ctx context.Context, account ChannelAccount, chatID string) (ClientBotRoute, error) {
	route, err := s.runtime.repository.ClientBotRouteByChat(ctx, account.ID, chatID)
	if err != nil {
		return ClientBotRoute{}, err
	}
	if route.State != "ready" || route.SelectedWorkspaceID == "" || (!route.ExpiresAt.IsZero() && route.ExpiresAt.Before(time.Now().UTC())) {
		_ = s.runtime.services.BotSessions.ClearClientRoute(ctx, domain.SystemActor(), account.ID, chatID)
		return ClientBotRoute{}, errors.New("client bot route is not ready")
	}
	return route, nil
}

func (s *Server) selectMasterByPhone(ctx context.Context, account ChannelAccount, chatID, rawPhone, profileName string, user *tgapi.User, contact *tgapi.Contact) error {
	normalized, err := normalizeMasterPhone(rawPhone)
	if err != nil {
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Не удалось распознать номер мастера. Введите полный номер, например +7 999 111-22-33.",
			Buttons: []TelegramInlineButton{
				{Text: "Повторить", CallbackData: "client:enter_master_phone"},
			},
		})
	}
	workspace, err := s.runtime.repository.WorkspaceByMasterPhone(ctx, normalized)
	if err != nil {
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Мастер с таким номером не найден.",
			Buttons: []TelegramInlineButton{
				{Text: "Повторить", CallbackData: "client:enter_master_phone"},
				{Text: "Связаться с человеком", CallbackData: "client:human"},
			},
		})
	}
	profile, err := s.runtime.repository.MasterProfile(ctx, workspace.ID)
	if err != nil {
		return err
	}
	if !profile.TelegramEnabled {
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "У этого мастера Telegram-канал ещё не настроен. Укажите другой номер.",
			Buttons: []TelegramInlineButton{
				{Text: "Повторить", CallbackData: "client:enter_master_phone"},
			},
		})
	}
	if _, err := s.runtime.services.BotSessions.StoreClientRoute(ctx, domain.SystemActor(), usecase.ClientBotRouteInput{
		ChannelAccountID:              account.ID,
		ExternalChatID:                chatID,
		SelectedWorkspaceID:           workspace.ID,
		SelectedMasterPhoneNormalized: normalized,
		State:                         "ready",
		ExpiresAt:                     time.Now().UTC().Add(30 * 24 * time.Hour),
	}); err != nil {
		return err
	}
	if _, err := s.runtime.repository.EnsureCustomerIdentity(ctx, workspace.ID, ChannelTelegram, chatID, InboundProfile{
		Name:     profileName,
		Username: usernameFromUser(user),
		Phone:    phoneFromContact(contact),
	}); err != nil {
		return err
	}
	return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
		ChatID:  chatID,
		Text:    fmt.Sprintf("Мастер выбран: %s.\nТеперь можно открыть календарь для записи или написать сообщение.", workspace.Name),
		Buttons: s.clientBotButtons(account, chatID, workspace.ID, true),
	})
}

func (s *Server) answerTelegramCallback(ctx context.Context, account ChannelAccount, callbackID, text string) {
	if strings.TrimSpace(callbackID) == "" || strings.TrimSpace(account.EncryptedToken) == "" {
		return
	}
	token, err := decryptString(s.cfg.EncryptionSecret, account.EncryptedToken)
	if err != nil {
		return
	}
	_ = s.runtime.telegram.AnswerCallback(ctx, token, tgapi.AnswerCallbackQueryRequest{
		CallbackQueryID: callbackID,
		Text:            text,
	})
}

func (s *Server) notifyOperatorsAboutConversation(ctx context.Context, conversation Conversation, customer Customer) {
	if conversation.Status != ConversationHuman && conversation.Status != ConversationNew {
		return
	}
	account, err := s.runtime.repository.ChannelAccountByKind(ctx, conversation.WorkspaceID, ChannelKindTelegramOperator)
	if err != nil {
		return
	}
	bindings, err := s.runtime.repository.ActiveOperatorBindings(ctx, conversation.WorkspaceID)
	if err != nil {
		return
	}
	text := fmt.Sprintf("🔔 Новое обращение [%s]\n\n%s:\n%s", conversation.Provider, customer.Name, conversation.LastMessageText)
	for _, binding := range bindings {
		_ = s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, conversation.ID, "", TelegramOutboundPayload{
			ChatID: binding.TelegramChatID,
			Text:   text,
			Buttons: []TelegramInlineButton{
				{Text: "Открыть", CallbackData: "dlg:open:" + conversation.ID},
				{Text: "Ответить", CallbackData: "dlg:reply:" + conversation.ID},
				{Text: "Слоты", CallbackData: "slot:list:" + conversation.ID},
			},
		})
	}
}

func (s *Server) handleTelegramClientUpdate(ctx context.Context, account ChannelAccount, update tgapi.Update) error {
	chatID := ""
	messageID := int64(0)
	callbackID := ""
	if update.Message != nil {
		chatID = strconv.FormatInt(update.Message.Chat.ID, 10)
		messageID = update.Message.MessageID
	}
	if update.CallbackQuery != nil {
		callbackID = update.CallbackQuery.ID
		if update.CallbackQuery.Message != nil {
			chatID = strconv.FormatInt(update.CallbackQuery.Message.Chat.ID, 10)
			messageID = update.CallbackQuery.Message.MessageID
		}
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message == nil {
		log.Printf("telegram inbound bot=client stage=skip reason=callback_without_message update_id=%d callback_id=%q value=%q", update.UpdateID, callbackID, telegramUpdateDebugValue(update))
		s.answerTelegramCallback(ctx, account, callbackID, "")
		return nil
	}
	if chatID == "" {
		return nil
	}
	log.Printf("telegram inbound bot=client update_id=%d chat_id=%s message_id=%d callback_id=%q value=%q", update.UpdateID, chatID, messageID, callbackID, telegramUpdateDebugValue(update))
	freshDelivery, err := s.claimTelegramInboundDelivery(ctx, account.ID, ChannelKindTelegramClient, chatID, messageID, callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=client stage=redis_claim outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, chatID, err)
		return err
	}
	if !freshDelivery {
		log.Printf("telegram inbound bot=client stage=redis_claim outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, chatID, messageID, callbackID)
		return nil
	}
	isNew, err := s.runtime.repository.MarkTelegramUpdateProcessed(ctx, account.WorkspaceID, account.ID, ChannelKindTelegramClient, update.UpdateID, chatID, messageID, callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=client stage=db_dedup outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, chatID, err)
		return err
	}
	if !isNew {
		log.Printf("telegram inbound bot=client stage=db_dedup outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, chatID, messageID, callbackID)
		return nil
	}

	if update.CallbackQuery != nil {
		s.answerTelegramCallback(ctx, account, update.CallbackQuery.ID, "")
		return s.handleTelegramClientCallback(ctx, account, update)
	}
	if update.Message == nil {
		return nil
	}

	text := strings.TrimSpace(update.Message.Text)
	profileName := ""
	if update.Message.From != nil {
		profileName = strings.TrimSpace(strings.TrimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName))
	}
	if strings.EqualFold(text, "/start") {
		if account.AccountScope == ChannelAccountScopeGlobal {
			_ = s.runtime.services.BotSessions.ClearClientRoute(ctx, domain.SystemActor(), account.ID, chatID)
			return s.promptTelegramMasterPhone(ctx, account, chatID)
		}
		if _, err := s.runtime.repository.EnsureCustomerIdentity(ctx, account.WorkspaceID, ChannelTelegram, chatID, InboundProfile{
			Name:     profileName,
			Username: usernameFromUser(update.Message.From),
			Phone:    phoneFromContact(update.Message.Contact),
		}); err != nil {
			return err
		}
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
			ChatID:  chatID,
			Text:    "Здравствуйте!\nПомогу записаться или ответить на вопросы.",
			Buttons: s.clientBotButtons(account, chatID, account.WorkspaceID, false),
		})
	}

	if account.AccountScope == ChannelAccountScopeGlobal {
		if strings.EqualFold(text, "сменить мастера") {
			_ = s.runtime.services.BotSessions.ClearClientRoute(ctx, domain.SystemActor(), account.ID, chatID)
			return s.promptTelegramMasterPhone(ctx, account, chatID)
		}
		route, err := s.selectedClientRoute(ctx, account, chatID)
		if err != nil {
			return s.selectMasterByPhone(ctx, account, chatID, text, profileName, update.Message.From, update.Message.Contact)
		}
		result, err := s.runtime.services.Inbox.ReceiveInboundMessageForWorkspace(ctx, route.SelectedWorkspaceID, inboundUsecaseInput(
			ChannelTelegram,
			account.ID,
			chatID,
			strconv.FormatInt(update.Message.MessageID, 10),
			text,
			time.Unix(update.Message.Date, 0).UTC(),
			InboundProfile{
				Name:     profileName,
				Username: usernameFromUser(update.Message.From),
				Phone:    phoneFromContact(update.Message.Contact),
			},
		))
		if err != nil {
			return err
		}
		log.Printf(
			"telegram inbound bot=client stage=inbox scope=workspace chat_id=%s external_message_id=%q stored=%t workspace_id=%s conversation_id=%s message_id=%s",
			chatID,
			strconv.FormatInt(update.Message.MessageID, 10),
			result.Stored,
			result.WorkspaceID,
			result.ConversationID,
			result.MessageID,
		)
		if !result.Stored {
			return nil
		}
		conversation, _, customer, detailErr := s.runtime.repository.ConversationDetail(ctx, result.WorkspaceID, result.ConversationID)
		if detailErr == nil {
			_ = s.publishEvent(ctx, SSEEvent{Type: EventMessageNew, Data: Message{ID: result.MessageID, ConversationID: result.ConversationID}})
			_ = s.publishDashboard(ctx, result.WorkspaceID)
			s.notifyOperatorsAboutConversation(ctx, conversation, customer)
		}
		return nil
	}

	result, err := s.runtime.services.Inbox.ReceiveInboundMessage(ctx, inboundUsecaseInput(
		ChannelTelegram,
		account.ID,
		chatID,
		strconv.FormatInt(update.Message.MessageID, 10),
		text,
		time.Unix(update.Message.Date, 0).UTC(),
		InboundProfile{
			Name:     profileName,
			Username: usernameFromUser(update.Message.From),
			Phone:    phoneFromContact(update.Message.Contact),
		},
	))
	if err != nil {
		return err
	}
	log.Printf(
		"telegram inbound bot=client stage=inbox scope=account chat_id=%s external_message_id=%q stored=%t workspace_id=%s conversation_id=%s message_id=%s",
		chatID,
		strconv.FormatInt(update.Message.MessageID, 10),
		result.Stored,
		result.WorkspaceID,
		result.ConversationID,
		result.MessageID,
	)
	if !result.Stored {
		return nil
	}
	conversation, _, customer, detailErr := s.runtime.repository.ConversationDetail(ctx, result.WorkspaceID, result.ConversationID)
	if detailErr == nil {
		_ = s.publishEvent(ctx, SSEEvent{Type: EventMessageNew, Data: Message{ID: result.MessageID, ConversationID: result.ConversationID}})
		_ = s.publishDashboard(ctx, result.WorkspaceID)
		s.notifyOperatorsAboutConversation(ctx, conversation, customer)
	}
	return nil
}

func (s *Server) handleTelegramClientCallback(ctx context.Context, account ChannelAccount, update tgapi.Update) error {
	if update.CallbackQuery == nil || update.CallbackQuery.Message == nil {
		return nil
	}
	data := strings.TrimSpace(update.CallbackQuery.Data)
	chatID := strconv.FormatInt(update.CallbackQuery.Message.Chat.ID, 10)
	if data == "" || chatID == "" {
		return nil
	}
	freshAction, err := s.claimTelegramCallbackAction(ctx, account.ID, ChannelKindTelegramClient, chatID, update.CallbackQuery.Message.MessageID, data)
	if err != nil {
		log.Printf("telegram inbound bot=client stage=callback_claim outcome=error chat_id=%s message_id=%d command=%q error=%v", chatID, update.CallbackQuery.Message.MessageID, data, err)
		return err
	}
	if !freshAction {
		log.Printf("telegram inbound bot=client stage=callback_claim outcome=duplicate chat_id=%s message_id=%d command=%q", chatID, update.CallbackQuery.Message.MessageID, data)
		return nil
	}
	profile := InboundProfile{
		Name:     strings.TrimSpace(update.CallbackQuery.From.FirstName + " " + update.CallbackQuery.From.LastName),
		Username: update.CallbackQuery.From.Username,
	}
	targetWorkspaceID := account.WorkspaceID
	if account.AccountScope == ChannelAccountScopeGlobal {
		switch data {
		case "client:change_master", "client:enter_master_phone":
			_ = s.runtime.services.BotSessions.ClearClientRoute(ctx, domain.SystemActor(), account.ID, chatID)
			return s.promptTelegramMasterPhone(ctx, account, chatID)
		}
		route, err := s.selectedClientRoute(ctx, account, chatID)
		if err != nil {
			return s.promptTelegramMasterPhone(ctx, account, chatID)
		}
		targetWorkspaceID = route.SelectedWorkspaceID
	}
	switch {
	case data == "client:book", data == "client:slots":
		return s.sendTelegramCalendarPrompt(ctx, account, chatID, targetWorkspaceID)
	case data == "client:prices":
		externalMessageID := telegramCallbackExternalMessageID(update)
		input := inboundUsecaseInput(
			ChannelTelegram,
			account.ID,
			chatID,
			externalMessageID,
			"Сколько стоит?",
			time.Now().UTC(),
			profile,
		)
		if account.AccountScope == ChannelAccountScopeGlobal {
			result, err := s.runtime.services.Inbox.ReceiveInboundMessageForWorkspace(ctx, targetWorkspaceID, input)
			if err == nil {
				log.Printf(
					"telegram inbound bot=client stage=inbox scope=callback command=%q chat_id=%s external_message_id=%q stored=%t workspace_id=%s conversation_id=%s message_id=%s",
					data,
					chatID,
					externalMessageID,
					result.Stored,
					result.WorkspaceID,
					result.ConversationID,
					result.MessageID,
				)
			}
			return err
		}
		result, err := s.runtime.services.Inbox.ReceiveInboundMessage(ctx, input)
		if err == nil {
			log.Printf(
				"telegram inbound bot=client stage=inbox scope=callback command=%q chat_id=%s external_message_id=%q stored=%t workspace_id=%s conversation_id=%s message_id=%s",
				data,
				chatID,
				externalMessageID,
				result.Stored,
				result.WorkspaceID,
				result.ConversationID,
				result.MessageID,
			)
		}
		return err
	case data == "client:address":
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Адрес уточнит оператор в этом чате. Если адрес уже есть в FAQ, добавьте его туда для автоответа.",
		})
	case data == "client:human":
		externalMessageID := telegramCallbackExternalMessageID(update)
		input := inboundUsecaseInput(
			ChannelTelegram,
			account.ID,
			chatID,
			externalMessageID,
			"Хочу связаться с человеком",
			time.Now().UTC(),
			profile,
		)
		var (
			result usecase.InboundResult
			err    error
		)
		if account.AccountScope == ChannelAccountScopeGlobal {
			result, err = s.runtime.services.Inbox.ReceiveInboundMessageForWorkspace(ctx, targetWorkspaceID, input)
		} else {
			result, err = s.runtime.services.Inbox.ReceiveInboundMessage(ctx, input)
		}
		if err != nil {
			return err
		}
		log.Printf(
			"telegram inbound bot=client stage=inbox scope=callback command=%q chat_id=%s external_message_id=%q stored=%t workspace_id=%s conversation_id=%s message_id=%s",
			data,
			chatID,
			externalMessageID,
			result.Stored,
			result.WorkspaceID,
			result.ConversationID,
			result.MessageID,
		)
		if !result.Stored {
			return nil
		}
		if conversation, _, customer, detailErr := s.runtime.repository.ConversationDetail(ctx, result.WorkspaceID, result.ConversationID); detailErr == nil {
			_ = s.publishEvent(ctx, SSEEvent{Type: EventMessageNew, Data: Message{ID: result.MessageID, ConversationID: result.ConversationID}})
			_ = s.publishDashboard(ctx, result.WorkspaceID)
			s.notifyOperatorsAboutConversation(ctx, conversation, customer)
		}
		return nil
	case strings.HasPrefix(data, "slot:select:"):
		conversation, customer, err := s.runtime.repository.ConversationByExternalChat(ctx, targetWorkspaceID, ChannelTelegram, chatID)
		if err != nil {
			return err
		}
		slotID := strings.TrimPrefix(data, "slot:select:")
		actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}
		booking, err := s.runtime.services.Bookings.CreateBooking(ctx, actor, usecase.CreateBookingInput{
			WorkspaceID:    targetWorkspaceID,
			CustomerID:     customer.ID,
			DailySlotID:    slotID,
			Status:         "pending",
			Notes:          "Создано через Telegram client bot",
			ConversationID: conversation.ID,
		})
		if err != nil {
			if strings.Contains(err.Error(), "slot unavailable") {
				return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
					ChatID: chatID,
					Text:   "Этот слот уже недоступен. Показываю свободные варианты заново.",
				})
			}
			return err
		}
		if _, err := s.runtime.services.BotSessions.StartSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}, usecase.BotSessionInput{
			WorkspaceID: targetWorkspaceID,
			BotKind:     string(ChannelKindTelegramClient),
			Scope:       string(BotSessionScopeClient),
			ActorType:   string(BotSessionActorCustomer),
			ActorID:     customer.ID,
			State:       "awaiting_confirm_booking",
			ExpiresAt:   time.Now().UTC().Add(30 * time.Minute),
		}, map[string]string{"bookingId": booking.ID, "conversationId": conversation.ID, "dailySlotId": slotID}); err != nil {
			return err
		}
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, conversation.ID, "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Подтвердить выбранное время?",
			Buttons: []TelegramInlineButton{
				{Text: "Подтвердить", CallbackData: "booking:confirm:" + slotID},
				{Text: "Отмена", CallbackData: "booking:cancel"},
			},
		})
	case strings.HasPrefix(data, "booking:confirm:"):
		conversation, customer, err := s.runtime.repository.ConversationByExternalChat(ctx, targetWorkspaceID, ChannelTelegram, chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
					ChatID: chatID,
					Text:   "Не удалось найти активный диалог. Откройте календарь заново и выберите актуальный слот.",
				})
			}
			return err
		}
		session, err := s.runtime.repository.LoadBotSession(ctx, targetWorkspaceID, BotSessionScopeClient, BotSessionActorCustomer, customer.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
					ChatID: chatID,
					Text:   "Это действие уже обработано или устарело. Откройте календарь заново и выберите актуальный слот.",
				})
			}
			return err
		}
		var payload struct {
			BookingID string `json:"bookingId"`
		}
		if err := json.Unmarshal([]byte(session.Payload), &payload); err != nil {
			_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}, targetWorkspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
			return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
				ChatID: chatID,
				Text:   "Не удалось восстановить состояние бронирования. Откройте календарь заново и выберите слот еще раз.",
			})
		}
		if strings.TrimSpace(payload.BookingID) == "" {
			_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}, targetWorkspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
			return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
				ChatID: chatID,
				Text:   "Не удалось восстановить бронирование. Откройте календарь заново и выберите время еще раз.",
			})
		}
		actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}
		booking, err := s.runtime.services.Bookings.ConfirmBooking(ctx, actor, targetWorkspaceID, payload.BookingID, 0, conversation.ID)
		if err != nil {
			if strings.Contains(err.Error(), "slot unavailable") {
				return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
					ChatID: chatID,
					Text:   "Слот уже недоступен. Показываю доступные окна заново.",
				})
			}
			return err
		}
		_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}, targetWorkspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
		s.notifyOperatorsAboutConversation(ctx, conversation, customer)
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   fmt.Sprintf("Запись подтверждена: %s.", booking.StartsAt.In(time.Local).Format("02.01 15:04")),
		})
	case data == "booking:cancel":
		conversation, customer, err := s.runtime.repository.ConversationByExternalChat(ctx, targetWorkspaceID, ChannelTelegram, chatID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
					ChatID: chatID,
					Text:   "Активное бронирование не найдено. Откройте календарь заново и выберите слот еще раз.",
				})
			}
			return err
		}
		session, err := s.runtime.repository.LoadBotSession(ctx, targetWorkspaceID, BotSessionScopeClient, BotSessionActorCustomer, customer.ID)
		if err == nil {
			var payload struct {
				BookingID string `json:"bookingId"`
			}
			if json.Unmarshal([]byte(session.Payload), &payload) == nil && strings.TrimSpace(payload.BookingID) != "" {
				actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}
				_, _ = s.runtime.services.Bookings.CancelBooking(ctx, actor, targetWorkspaceID, payload.BookingID, conversation.ID)
			} else {
				_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}, targetWorkspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
				return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
					ChatID: chatID,
					Text:   "Не удалось восстановить состояние бронирования. Откройте календарь заново и выберите слот еще раз.",
				})
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: targetWorkspaceID}, targetWorkspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendText, conversation.ID, "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Действие отменено.",
		})
	}
	return nil
}

func (s *Server) handleTelegramOperatorUpdate(ctx context.Context, operatorAccount ChannelAccount, update tgapi.Update) error {
	chatID, userID, text, err := parseOperatorUpdate(update)
	if err != nil {
		return nil
	}
	messageID := messageIDFromUpdate(update)
	callbackID := callbackIDFromUpdate(update)
	if update.CallbackQuery != nil && update.CallbackQuery.Message == nil {
		log.Printf("telegram inbound bot=operator stage=skip reason=callback_without_message update_id=%d user_id=%s callback_id=%q value=%q", update.UpdateID, userID, callbackID, telegramUpdateDebugValue(update))
		s.answerTelegramCallback(ctx, operatorAccount, callbackID, "")
		return nil
	}
	log.Printf("telegram inbound bot=operator update_id=%d chat_id=%s user_id=%s message_id=%d callback_id=%q value=%q", update.UpdateID, chatID, userID, messageID, callbackID, telegramUpdateDebugValue(update))
	freshDelivery, err := s.claimTelegramInboundDelivery(ctx, operatorAccount.ID, ChannelKindTelegramOperator, chatID, messageID, callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=operator stage=redis_claim outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, chatID, err)
		return err
	}
	if !freshDelivery {
		log.Printf("telegram inbound bot=operator stage=redis_claim outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, chatID, messageID, callbackID)
		return nil
	}

	if strings.HasPrefix(text, "/start ") {
		code := strings.TrimSpace(strings.TrimPrefix(text, "/start "))
		if _, err := s.runtime.services.OperatorLink.LinkTelegram(ctx, code, userID, chatID); err != nil {
			return s.enqueueTelegramOutbound(ctx, operatorAccount, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
				ChatID: chatID,
				Text:   "Не удалось привязать бота: " + err.Error(),
			})
		}
		return s.enqueueTelegramOutbound(ctx, operatorAccount, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Бот привязан. Доступны основные разделы.",
			Buttons: []TelegramInlineButton{
				{Text: "Дашборд", CallbackData: "/dashboard"},
				{Text: "Диалоги", CallbackData: "/dialogs"},
				{Text: "Слоты", CallbackData: "/slots"},
				{Text: "Настройки", CallbackData: "/settings"},
			},
		})
	}

	binding, err := s.runtime.repository.ActiveOperatorBindingByTelegramChat(ctx, chatID)
	if err != nil {
		return s.enqueueTelegramOutbound(ctx, operatorAccount, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Вы не привязаны. Откройте deep link из CRM или отправьте link code.",
		})
	}

	if update.CallbackQuery != nil {
		s.answerTelegramCallback(ctx, operatorAccount, update.CallbackQuery.ID, "")
		command := normalizeOperatorCommand(update.CallbackQuery.Data)
		freshAction, err := s.claimTelegramCallbackAction(ctx, operatorAccount.ID, ChannelKindTelegramOperator, chatID, update.CallbackQuery.Message.MessageID, command)
		if err != nil {
			log.Printf("telegram inbound bot=operator stage=callback_claim outcome=error chat_id=%s message_id=%d command=%q error=%v", chatID, update.CallbackQuery.Message.MessageID, command, err)
			return err
		}
		if !freshAction {
			log.Printf("telegram inbound bot=operator stage=callback_claim outcome=duplicate chat_id=%s message_id=%d command=%q", chatID, update.CallbackQuery.Message.MessageID, command)
			return nil
		}
	}

	isNew, err := s.runtime.repository.MarkTelegramUpdateProcessed(ctx, binding.WorkspaceID, operatorAccount.ID, ChannelKindTelegramOperator, update.UpdateID, chatID, messageID, callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=operator stage=db_dedup outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, chatID, err)
		return err
	}
	if !isNew {
		log.Printf("telegram inbound bot=operator stage=db_dedup outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, chatID, messageID, callbackID)
		return nil
	}
	if text == "/start" || text == "Отмена" {
		_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: binding.WorkspaceID, UserID: binding.UserID}, binding.WorkspaceID, string(BotSessionScopeOperator), string(BotSessionActorUser), binding.UserID)
	}
	command := normalizeOperatorCommand(text)
	session, sessionErr := s.runtime.repository.LoadBotSession(ctx, binding.WorkspaceID, BotSessionScopeOperator, BotSessionActorUser, binding.UserID)
	if sessionErr == nil {
		if handled, followUp, err := s.handleOperatorSession(ctx, binding, session, command); err == nil && handled {
			return s.enqueueBotOutboundMessages(ctx, operatorAccount, followUp)
		}
	}
	responses, err := s.handleOperatorCommand(ctx, binding, command)
	if err != nil {
		return s.enqueueTelegramOutbound(ctx, operatorAccount, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   "Команда не обработана: " + err.Error(),
		})
	}
	return s.enqueueBotOutboundMessages(ctx, operatorAccount, responses)
}

func (s *Server) enqueueBotOutboundMessages(ctx context.Context, account ChannelAccount, messages []BotOutboundMessage) error {
	for _, message := range messages {
		kind := OutboundKindTelegramSendText
		payload := TelegramOutboundPayload{
			ChatID: message.ChatID,
			Text:   message.Text,
		}
		if len(message.Buttons) > 0 {
			kind = OutboundKindTelegramSendInline
			payload.Buttons = telegramButtonsFromCommands(message.Buttons)
		}
		if err := s.enqueueTelegramOutbound(ctx, account, kind, message.ConversationID, "", payload); err != nil {
			return err
		}
	}
	return nil
}

func parseOperatorUpdate(update tgapi.Update) (chatID, userID, text string, err error) {
	switch {
	case update.Message != nil:
		chatID = strconv.FormatInt(update.Message.Chat.ID, 10)
		if update.Message.From != nil {
			userID = strconv.FormatInt(update.Message.From.ID, 10)
		}
		text = strings.TrimSpace(update.Message.Text)
	case update.CallbackQuery != nil:
		if update.CallbackQuery.Message != nil {
			chatID = strconv.FormatInt(update.CallbackQuery.Message.Chat.ID, 10)
		}
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

func normalizeOperatorCommand(text string) string {
	switch {
	case strings.HasPrefix(text, "dlg:open:"):
		return "dialog:" + strings.TrimPrefix(text, "dlg:open:")
	case strings.HasPrefix(text, "dlg:reply:"):
		return "reply:" + strings.TrimPrefix(text, "dlg:reply:")
	case strings.HasPrefix(text, "slot:list:"):
		return "slots:" + strings.TrimPrefix(text, "slot:list:")
	case strings.HasPrefix(text, "slot:pick:"):
		parts := strings.Split(text, ":")
		if len(parts) == 4 {
			return "pickslot:" + parts[2] + ":" + parts[3]
		}
	case strings.HasPrefix(text, "dlg:close:"):
		return "close:" + strings.TrimPrefix(text, "dlg:close:")
	case text == "set:auto:on":
		return "/auto_on"
	case text == "set:auto:off":
		return "/auto_off"
	case strings.HasPrefix(text, "dlg:take:"):
		return "take:" + strings.TrimPrefix(text, "dlg:take:")
	case strings.HasPrefix(text, "dlg:auto:"):
		return "auto:" + strings.TrimPrefix(text, "dlg:auto:")
	}
	return text
}

func messageIDFromUpdate(update tgapi.Update) int64 {
	if update.Message != nil {
		return update.Message.MessageID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		return update.CallbackQuery.Message.MessageID
	}
	return 0
}

func callbackIDFromUpdate(update tgapi.Update) string {
	if update.CallbackQuery != nil {
		return update.CallbackQuery.ID
	}
	return ""
}

func usernameFromUser(user *tgapi.User) string {
	if user == nil {
		return ""
	}
	return user.Username
}

func phoneFromContact(contact *tgapi.Contact) string {
	if contact == nil {
		return ""
	}
	return contact.PhoneNumber
}

func telegramCallbackExternalMessageID(update tgapi.Update) string {
	if update.CallbackQuery == nil {
		return ""
	}
	data := strings.TrimSpace(update.CallbackQuery.Data)
	if data == "" {
		return ""
	}
	if update.CallbackQuery.Message != nil {
		chatID := strconv.FormatInt(update.CallbackQuery.Message.Chat.ID, 10)
		return fmt.Sprintf("cbqmsg:%s:%d:%s", chatID, update.CallbackQuery.Message.MessageID, data)
	}
	callbackID := strings.TrimSpace(update.CallbackQuery.ID)
	if callbackID == "" {
		return ""
	}
	return "cbq:" + callbackID
}

func inboundUsecaseInput(provider ChannelProvider, accountID, externalChatID, externalMessageID, text string, timestamp time.Time, profile InboundProfile) usecase.InboundInput {
	return usecase.InboundInput{
		Provider:          string(provider),
		ChannelAccountID:  accountID,
		ExternalChatID:    externalChatID,
		ExternalMessageID: externalMessageID,
		Text:              text,
		Timestamp:         timestamp,
		Profile: usecase.InboundProfile{
			Name:     profile.Name,
			Username: profile.Username,
			Phone:    profile.Phone,
		},
	}
}
