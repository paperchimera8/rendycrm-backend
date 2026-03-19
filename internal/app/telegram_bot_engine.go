package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
	tgapi "github.com/vital/rendycrm-app/internal/telegram"
	"github.com/vital/rendycrm-app/internal/usecase"
)

const (
	botEngineHTTPTimeout        = 5 * time.Second
	botEngineHTTPBodyLimitBytes = 1 << 20
	botEngineClientRouteTTL     = 30 * 24 * time.Hour
	botEngineClientBookingTTL   = 30 * time.Minute
	botEngineOperatorSessionTTL = 15 * time.Minute
)

type botEngineUnavailableError struct {
	err error
}

func (e *botEngineUnavailableError) Error() string {
	if e == nil || e.err == nil {
		return "bot engine unavailable"
	}
	return e.err.Error()
}

func (e *botEngineUnavailableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func wrapBotEngineUnavailable(err error) error {
	if err == nil {
		return nil
	}
	return &botEngineUnavailableError{err: err}
}

type botEngineClient struct {
	baseURL    string
	httpClient *http.Client
}

func newBotEngineClient(baseURL string) *botEngineClient {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil
	}
	return &botEngineClient{
		baseURL:    trimmed,
		httpClient: &http.Client{Timeout: botEngineHTTPTimeout},
	}
}

type botEngineButton struct {
	Text   string `json:"text"`
	Action string `json:"action"`
}

type botEngineEffect struct {
	Type           string            `json:"type"`
	Text           string            `json:"text,omitempty"`
	Buttons        []botEngineButton `json:"buttons,omitempty"`
	Intent         string            `json:"intent,omitempty"`
	WorkspaceID    string            `json:"workspaceId,omitempty"`
	WorkspaceName  string            `json:"workspaceName,omitempty"`
	ConversationID string            `json:"conversationId,omitempty"`
	SlotID         string            `json:"slotId,omitempty"`
	SlotLabel      string            `json:"slotLabel,omitempty"`
	Amount         int               `json:"amount,omitempty"`
	CustomerID     string            `json:"customerId,omitempty"`
	Enabled        bool              `json:"enabled,omitempty"`
	UserID         string            `json:"userId,omitempty"`
	ChatID         string            `json:"chatId,omitempty"`
	MasterPhone    string            `json:"masterPhone,omitempty"`
	Bot            string            `json:"bot,omitempty"`
	Message        string            `json:"message,omitempty"`
}

type botEngineClientRouteState struct {
	Kind          string `json:"kind"`
	PromptedAt    string `json:"promptedAt,omitempty"`
	WorkspaceID   string `json:"workspaceId,omitempty"`
	WorkspaceName string `json:"workspaceName,omitempty"`
	MasterPhone   string `json:"masterPhone,omitempty"`
}

type botEngineClientBookingState struct {
	Kind        string `json:"kind"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	SlotID      string `json:"slotId,omitempty"`
	SlotLabel   string `json:"slotLabel,omitempty"`
}

type botEngineClientSession struct {
	Route          botEngineClientRouteState   `json:"route"`
	Booking        botEngineClientBookingState `json:"booking"`
	RecentEventIDs []string                    `json:"recentEventIds,omitempty"`
}

type botEngineSlotOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type botEngineClientMasterDirectoryEntry struct {
	WorkspaceID     string `json:"workspaceId"`
	WorkspaceName   string `json:"workspaceName"`
	MasterPhone     string `json:"masterPhone"`
	TelegramEnabled bool   `json:"telegramEnabled"`
}

type botEngineClientFixedWorkspace struct {
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	MasterPhone   string `json:"masterPhone,omitempty"`
}

type botEngineClientContext struct {
	Mode             string                                `json:"mode"`
	Masters          []botEngineClientMasterDirectoryEntry `json:"masters"`
	SlotsByWorkspace map[string][]botEngineSlotOption      `json:"slotsByWorkspace"`
	FixedWorkspace   *botEngineClientFixedWorkspace        `json:"fixedWorkspace,omitempty"`
}

type botEngineClientEvent struct {
	Type    string     `json:"type"`
	EventID string     `json:"eventId,omitempty"`
	Payload string     `json:"payload,omitempty"`
	Text    string     `json:"text,omitempty"`
	Data    string     `json:"data,omitempty"`
	Now     *time.Time `json:"now,omitempty"`
}

type botEngineClientHandleRequest struct {
	Session *botEngineClientSession `json:"session,omitempty"`
	Event   botEngineClientEvent    `json:"event"`
	Context botEngineClientContext  `json:"context"`
}

type botEngineClientTransition struct {
	State   botEngineClientSession `json:"state"`
	Effects []botEngineEffect      `json:"effects"`
}

type botEngineOperatorBindingState struct {
	Kind        string `json:"kind"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	UserID      string `json:"userId,omitempty"`
	ChatID      string `json:"chatId,omitempty"`
}

type botEngineOperatorInteractionState struct {
	Kind           string `json:"kind"`
	ConversationID string `json:"conversationId,omitempty"`
	CustomerID     string `json:"customerId,omitempty"`
	SlotID         string `json:"slotId,omitempty"`
	SlotLabel      string `json:"slotLabel,omitempty"`
}

type botEngineOperatorSession struct {
	Binding           botEngineOperatorBindingState     `json:"binding"`
	Interaction       botEngineOperatorInteractionState `json:"interaction"`
	AutoReplyOverride *bool                             `json:"autoReplyOverride,omitempty"`
	RecentEventIDs    []string                          `json:"recentEventIds,omitempty"`
}

type botEngineOperatorLinkBinding struct {
	Code        string `json:"code"`
	WorkspaceID string `json:"workspaceId"`
	UserID      string `json:"userId"`
	ChatID      string `json:"chatId"`
}

type botEngineOperatorDashboard struct {
	TodayBookings int    `json:"todayBookings"`
	NewMessages   int    `json:"newMessages"`
	Revenue       int    `json:"revenue"`
	FreeSlots     int    `json:"freeSlots"`
	NextSlot      string `json:"nextSlot"`
}

type botEngineOperatorConversation struct {
	ID              string                `json:"id"`
	Title           string                `json:"title"`
	Provider        string                `json:"provider"`
	CustomerName    string                `json:"customerName"`
	CustomerPhone   string                `json:"customerPhone,omitempty"`
	CustomerID      string                `json:"customerId,omitempty"`
	Status          string                `json:"status"`
	LastMessageText string                `json:"lastMessageText"`
	UnreadCount     int                   `json:"unreadCount"`
	SlotOptions     []botEngineSlotOption `json:"slotOptions"`
}

type botEngineOperatorWeekSlot struct {
	Label     string `json:"label"`
	SlotCount int    `json:"slotCount"`
}

type botEngineOperatorSettings struct {
	AutoReply         bool     `json:"autoReply"`
	HandoffEnabled    bool     `json:"handoffEnabled"`
	TelegramChatLabel string   `json:"telegramChatLabel"`
	WebhookURL        string   `json:"webhookUrl"`
	FAQQuestions      []string `json:"faqQuestions"`
}

type botEngineOperatorWorkspace struct {
	ID            string                          `json:"id"`
	Name          string                          `json:"name"`
	Dashboard     botEngineOperatorDashboard      `json:"dashboard"`
	Conversations []botEngineOperatorConversation `json:"conversations"`
	WeekSlots     []botEngineOperatorWeekSlot     `json:"weekSlots"`
	Settings      botEngineOperatorSettings       `json:"settings"`
}

type botEngineOperatorContext struct {
	LinkBindings []botEngineOperatorLinkBinding `json:"linkBindings"`
	Workspaces   []botEngineOperatorWorkspace   `json:"workspaces"`
}

type botEngineOperatorEvent struct {
	Type    string `json:"type"`
	EventID string `json:"eventId,omitempty"`
	Payload string `json:"payload,omitempty"`
	Text    string `json:"text,omitempty"`
	Data    string `json:"data,omitempty"`
}

type botEngineOperatorHandleRequest struct {
	Session *botEngineOperatorSession `json:"session,omitempty"`
	Event   botEngineOperatorEvent    `json:"event"`
	Context botEngineOperatorContext  `json:"context"`
}

type botEngineOperatorTransition struct {
	State   botEngineOperatorSession `json:"state"`
	Effects []botEngineEffect        `json:"effects"`
}

func (c *botEngineClient) handleClient(ctx context.Context, request botEngineClientHandleRequest) (botEngineClientTransition, error) {
	var response botEngineClientTransition
	if err := c.postJSON(ctx, "/client/handle", request, &response); err != nil {
		return botEngineClientTransition{}, err
	}
	return response, nil
}

func (c *botEngineClient) handleOperator(ctx context.Context, request botEngineOperatorHandleRequest) (botEngineOperatorTransition, error) {
	var response botEngineOperatorTransition
	if err := c.postJSON(ctx, "/operator/handle", request, &response); err != nil {
		return botEngineOperatorTransition{}, err
	}
	return response, nil
}

func (c *botEngineClient) postJSON(ctx context.Context, path string, requestBody any, responseBody any) error {
	if c == nil || c.httpClient == nil || c.baseURL == "" {
		return errors.New("bot engine client is not configured")
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal bot engine request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create bot engine request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("do bot engine request: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8<<10))
		return fmt.Errorf("bot engine returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(io.LimitReader(response.Body, botEngineHTTPBodyLimitBytes)).Decode(responseBody); err != nil {
		return fmt.Errorf("decode bot engine response: %w", err)
	}
	return nil
}

func (s *Server) botEngineEnabled() bool {
	return s != nil && s.botEngine != nil
}

func (s *Server) handleTelegramClientUpdate(ctx context.Context, account ChannelAccount, update tgapi.Update) error {
	if !s.botEngineEnabled() {
		return s.handleTelegramClientUpdateLegacy(ctx, account, update)
	}
	if err := s.handleTelegramClientUpdateWithEngine(ctx, account, update, true); err != nil {
		var unavailable *botEngineUnavailableError
		if errors.As(err, &unavailable) {
			log.Printf("telegram inbound bot=client stage=engine outcome=fallback update_id=%d error=%v", update.UpdateID, unavailable)
			return s.handleTelegramClientUpdateLegacy(ctx, account, update)
		}
		return err
	}
	return nil
}

func (s *Server) handleTelegramClientCallback(ctx context.Context, account ChannelAccount, update tgapi.Update) error {
	if !s.botEngineEnabled() {
		return s.handleTelegramClientCallbackLegacy(ctx, account, update)
	}
	if err := s.handleTelegramClientUpdateWithEngine(ctx, account, update, false); err != nil {
		var unavailable *botEngineUnavailableError
		if errors.As(err, &unavailable) {
			log.Printf("telegram inbound bot=client stage=engine_callback outcome=fallback update_id=%d error=%v", update.UpdateID, unavailable)
			return s.handleTelegramClientCallbackLegacy(ctx, account, update)
		}
		return err
	}
	return nil
}

func (s *Server) handleTelegramOperatorUpdate(ctx context.Context, operatorAccount ChannelAccount, update tgapi.Update) error {
	if !s.botEngineEnabled() {
		return s.handleTelegramOperatorUpdateLegacy(ctx, operatorAccount, update)
	}
	if err := s.handleTelegramOperatorUpdateWithEngine(ctx, operatorAccount, update); err != nil {
		var unavailable *botEngineUnavailableError
		if errors.As(err, &unavailable) {
			log.Printf("telegram inbound bot=operator stage=engine outcome=fallback update_id=%d error=%v", update.UpdateID, unavailable)
			return s.handleTelegramOperatorUpdateLegacy(ctx, operatorAccount, update)
		}
		return err
	}
	return nil
}

func (s *Server) handleTelegramClientUpdateWithEngine(ctx context.Context, account ChannelAccount, update tgapi.Update, answerCallback bool) error {
	inbound, err := buildTelegramClientInbound(update)
	if err != nil {
		return nil
	}
	if inbound.callbackWithoutMessage {
		if answerCallback {
			s.answerTelegramCallback(ctx, account, inbound.callbackID, "")
		}
		log.Printf("telegram inbound bot=client stage=skip reason=callback_without_message update_id=%d callback_id=%q value=%q", update.UpdateID, inbound.callbackID, telegramUpdateDebugValue(update))
		return nil
	}
	if inbound.chatID == "" {
		return nil
	}

	log.Printf("telegram inbound bot=client stage=engine_prepare update_id=%d chat_id=%s message_id=%d callback_id=%q value=%q", update.UpdateID, inbound.chatID, inbound.messageID, inbound.callbackID, telegramUpdateDebugValue(update))

	if answerCallback && inbound.callbackID != "" {
		s.answerTelegramCallback(ctx, account, inbound.callbackID, "")
	}

	session, err := s.loadBotEngineClientSession(ctx, account, inbound.chatID)
	if err != nil {
		return err
	}
	contextState, err := s.buildBotEngineClientContext(ctx, account, session)
	if err != nil {
		return err
	}
	event := buildBotEngineClientEvent(update, inbound)

	transition, err := s.botEngine.handleClient(ctx, botEngineClientHandleRequest{
		Session: session,
		Event:   event,
		Context: contextState,
	})
	if err != nil {
		return wrapBotEngineUnavailable(err)
	}

	freshDelivery, err := s.claimTelegramInboundDelivery(ctx, account.ID, ChannelKindTelegramClient, inbound.chatID, inbound.messageID, inbound.callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=client stage=redis_claim outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, inbound.chatID, err)
		return err
	}
	if !freshDelivery {
		log.Printf("telegram inbound bot=client stage=redis_claim outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, inbound.chatID, inbound.messageID, inbound.callbackID)
		return nil
	}

	isNew, err := s.runtime.repository.MarkTelegramUpdateProcessed(ctx, account.WorkspaceID, account.ID, ChannelKindTelegramClient, update.UpdateID, inbound.chatID, inbound.messageID, inbound.callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=client stage=db_dedup outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, inbound.chatID, err)
		return err
	}
	if !isNew {
		log.Printf("telegram inbound bot=client stage=db_dedup outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, inbound.chatID, inbound.messageID, inbound.callbackID)
		return nil
	}

	if inbound.callbackData != "" {
		freshAction, err := s.claimTelegramCallbackAction(ctx, account.ID, ChannelKindTelegramClient, inbound.chatID, inbound.messageID, inbound.callbackData)
		if err != nil {
			log.Printf("telegram inbound bot=client stage=callback_claim outcome=error chat_id=%s message_id=%d command=%q error=%v", inbound.chatID, inbound.messageID, inbound.callbackData, err)
			return err
		}
		if !freshAction {
			log.Printf("telegram inbound bot=client stage=callback_claim outcome=duplicate chat_id=%s message_id=%d command=%q", inbound.chatID, inbound.messageID, inbound.callbackData)
			return nil
		}
	}

	return s.applyBotEngineClientTransition(ctx, account, inbound, transition)
}

func (s *Server) handleTelegramOperatorUpdateWithEngine(ctx context.Context, operatorAccount ChannelAccount, update tgapi.Update) error {
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

	if strings.HasPrefix(strings.TrimSpace(text), "/start ") {
		return s.handleTelegramOperatorUpdateLegacy(ctx, operatorAccount, update)
	}

	log.Printf("telegram inbound bot=operator stage=engine_prepare update_id=%d chat_id=%s user_id=%s message_id=%d callback_id=%q value=%q", update.UpdateID, chatID, userID, messageID, callbackID, telegramUpdateDebugValue(update))

	if callbackID != "" {
		s.answerTelegramCallback(ctx, operatorAccount, callbackID, "")
	}

	binding, err := s.runtime.repository.ActiveOperatorBindingByTelegramChat(ctx, chatID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		binding = OperatorBotBinding{}
	}

	contextState, workspaceState, err := s.buildBotEngineOperatorContext(ctx, binding)
	if err != nil {
		return err
	}
	session, err := s.loadBotEngineOperatorSession(ctx, binding, workspaceState)
	if err != nil {
		return err
	}

	event := botEngineOperatorEvent{
		Type: "message",
		Text: normalizeOperatorCommand(text),
	}
	if update.CallbackQuery != nil {
		event.Type = "callback"
		event.Data = normalizeOperatorCommand(update.CallbackQuery.Data)
	}
	if event.Type == "message" && strings.TrimSpace(text) == "" {
		return nil
	}

	transition, err := s.botEngine.handleOperator(ctx, botEngineOperatorHandleRequest{
		Session: session,
		Event:   event,
		Context: contextState,
	})
	if err != nil {
		return wrapBotEngineUnavailable(err)
	}

	freshDelivery, err := s.claimTelegramInboundDelivery(ctx, operatorAccount.ID, ChannelKindTelegramOperator, chatID, messageID, callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=operator stage=redis_claim outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, chatID, err)
		return err
	}
	if !freshDelivery {
		log.Printf("telegram inbound bot=operator stage=redis_claim outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, chatID, messageID, callbackID)
		return nil
	}

	isNew, err := s.runtime.repository.MarkTelegramUpdateProcessed(ctx, operatorAccount.WorkspaceID, operatorAccount.ID, ChannelKindTelegramOperator, update.UpdateID, chatID, messageID, callbackID)
	if err != nil {
		log.Printf("telegram inbound bot=operator stage=db_dedup outcome=error update_id=%d chat_id=%s error=%v", update.UpdateID, chatID, err)
		return err
	}
	if !isNew {
		log.Printf("telegram inbound bot=operator stage=db_dedup outcome=duplicate update_id=%d chat_id=%s message_id=%d callback_id=%q", update.UpdateID, chatID, messageID, callbackID)
		return nil
	}

	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
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

	return s.applyBotEngineOperatorTransition(ctx, operatorAccount, chatID, binding, transition)
}

type telegramClientInbound struct {
	chatID                 string
	messageID              int64
	callbackID             string
	callbackData           string
	callbackWithoutMessage bool
	text                   string
	profile                InboundProfile
	timestamp              time.Time
	externalMessageID      string
}

func buildTelegramClientInbound(update tgapi.Update) (telegramClientInbound, error) {
	inbound := telegramClientInbound{
		timestamp: time.Now().UTC(),
	}
	if update.Message != nil {
		inbound.chatID = strconv.FormatInt(update.Message.Chat.ID, 10)
		inbound.messageID = update.Message.MessageID
		inbound.text = strings.TrimSpace(update.Message.Text)
		inbound.timestamp = time.Unix(update.Message.Date, 0).UTC()
		if update.Message.From != nil {
			inbound.profile.Username = usernameFromUser(update.Message.From)
			inbound.profile.Name = strings.TrimSpace(strings.TrimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName))
		}
		inbound.profile.Phone = phoneFromContact(update.Message.Contact)
		inbound.externalMessageID = strconv.FormatInt(update.Message.MessageID, 10)
		return inbound, nil
	}
	if update.CallbackQuery != nil {
		inbound.callbackID = update.CallbackQuery.ID
		inbound.callbackData = strings.TrimSpace(update.CallbackQuery.Data)
		inbound.externalMessageID = telegramCallbackExternalMessageID(update)
		if update.CallbackQuery.From.Username != "" {
			inbound.profile.Username = update.CallbackQuery.From.Username
		}
		inbound.profile.Name = strings.TrimSpace(update.CallbackQuery.From.FirstName + " " + update.CallbackQuery.From.LastName)
		if update.CallbackQuery.Message == nil {
			inbound.callbackWithoutMessage = true
			return inbound, nil
		}
		inbound.chatID = strconv.FormatInt(update.CallbackQuery.Message.Chat.ID, 10)
		inbound.messageID = update.CallbackQuery.Message.MessageID
		return inbound, nil
	}
	return inbound, errors.New("empty telegram update")
}

func buildBotEngineClientEvent(update tgapi.Update, inbound telegramClientInbound) botEngineClientEvent {
	if update.CallbackQuery != nil {
		return botEngineClientEvent{
			Type: "callback",
			Data: inbound.callbackData,
			Now:  &inbound.timestamp,
		}
	}

	event := botEngineClientEvent{
		Type: "message",
		Text: inbound.text,
		Now:  &inbound.timestamp,
	}
	if payload, isStart := telegramStartPayload(inbound.text); isStart {
		event.Type = "start"
		event.Payload = telegramMasterPhoneFromStartPayload(payload)
		event.Text = ""
	}
	return event
}

func (s *Server) loadBotEngineClientSession(ctx context.Context, account ChannelAccount, chatID string) (*botEngineClientSession, error) {
	session := &botEngineClientSession{
		Route: botEngineClientRouteState{
			Kind:       "awaiting_master_phone",
			PromptedAt: time.Unix(0, 0).UTC().Format(time.RFC3339),
		},
		Booking: botEngineClientBookingState{Kind: "idle"},
	}

	if account.AccountScope == ChannelAccountScopeGlobal {
		route, err := s.runtime.repository.ClientBotRouteByChat(ctx, account.ID, chatID)
		switch {
		case err == nil:
			switch route.State {
			case "ready":
				workspace, workspaceErr := s.runtime.repository.WorkspaceByID(ctx, route.SelectedWorkspaceID)
				if workspaceErr != nil {
					if errors.Is(workspaceErr, sql.ErrNoRows) {
						break
					}
					return nil, workspaceErr
				}
				session.Route = botEngineClientRouteState{
					Kind:          "ready",
					WorkspaceID:   route.SelectedWorkspaceID,
					WorkspaceName: workspace.Name,
					MasterPhone:   firstNonEmptyString(route.SelectedMasterPhoneNormalized, workspace.MasterPhoneNormalized),
				}
			default:
				session.Route = botEngineClientRouteState{
					Kind:       "awaiting_master_phone",
					PromptedAt: route.UpdatedAt.UTC().Format(time.RFC3339),
				}
			}
		case errors.Is(err, sql.ErrNoRows):
		default:
			return nil, err
		}
	}

	bookingWorkspaceID := account.WorkspaceID
	if session.Route.Kind == "ready" && session.Route.WorkspaceID != "" {
		bookingWorkspaceID = session.Route.WorkspaceID
	}
	if strings.TrimSpace(bookingWorkspaceID) == "" {
		return session, nil
	}

	_, customer, err := s.runtime.repository.ConversationByExternalChat(ctx, bookingWorkspaceID, ChannelTelegram, chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session, nil
		}
		return nil, err
	}

	botSession, err := s.runtime.repository.LoadBotSession(ctx, bookingWorkspaceID, BotSessionScopeClient, BotSessionActorCustomer, customer.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session, nil
		}
		return nil, err
	}
	if botSession.State != "awaiting_confirm_booking" {
		return session, nil
	}

	var payload struct {
		BookingID   string `json:"bookingId"`
		DailySlotID string `json:"dailySlotId"`
		SlotLabel   string `json:"slotLabel"`
	}
	if err := json.Unmarshal([]byte(botSession.Payload), &payload); err != nil {
		return session, nil
	}

	slotLabel := strings.TrimSpace(payload.SlotLabel)
	if slotLabel == "" && payload.BookingID != "" {
		booking, bookingErr := s.runtime.repository.Booking(ctx, bookingWorkspaceID, payload.BookingID)
		if bookingErr == nil {
			slotLabel = formatTelegramSlotLabel(booking.StartsAt)
		}
	}
	if slotLabel == "" {
		slotLabel = payload.DailySlotID
	}

	session.Booking = botEngineClientBookingState{
		Kind:        "awaiting_confirmation",
		WorkspaceID: bookingWorkspaceID,
		SlotID:      payload.DailySlotID,
		SlotLabel:   slotLabel,
	}
	return session, nil
}

func (s *Server) buildBotEngineClientContext(ctx context.Context, account ChannelAccount, session *botEngineClientSession) (botEngineClientContext, error) {
	contextState := botEngineClientContext{
		Mode:             "workspace",
		Masters:          []botEngineClientMasterDirectoryEntry{},
		SlotsByWorkspace: map[string][]botEngineSlotOption{},
	}

	if account.AccountScope == ChannelAccountScopeGlobal {
		contextState.Mode = "global"
		directory, err := s.runtime.repository.ClientBotMasterDirectory(ctx)
		if err != nil {
			return botEngineClientContext{}, err
		}
		contextState.Masters = make([]botEngineClientMasterDirectoryEntry, 0, len(directory))
		for _, item := range directory {
			contextState.Masters = append(contextState.Masters, botEngineClientMasterDirectoryEntry{
				WorkspaceID:     item.WorkspaceID,
				WorkspaceName:   item.WorkspaceName,
				MasterPhone:     item.MasterPhone,
				TelegramEnabled: item.TelegramEnabled,
			})
		}
		if session != nil && session.Route.Kind == "ready" && session.Route.WorkspaceID != "" {
			slots, err := s.loadBotEngineSlotOptions(ctx, session.Route.WorkspaceID)
			if err != nil {
				return botEngineClientContext{}, err
			}
			contextState.SlotsByWorkspace[session.Route.WorkspaceID] = slots
		}
		return contextState, nil
	}

	workspace, err := s.runtime.repository.WorkspaceByID(ctx, account.WorkspaceID)
	if err != nil {
		return botEngineClientContext{}, err
	}
	profile, err := s.runtime.repository.MasterProfile(ctx, account.WorkspaceID)
	if err != nil {
		return botEngineClientContext{}, err
	}
	contextState.FixedWorkspace = &botEngineClientFixedWorkspace{
		WorkspaceID:   workspace.ID,
		WorkspaceName: workspace.Name,
		MasterPhone:   profile.MasterPhoneNormalized,
	}
	slots, err := s.loadBotEngineSlotOptions(ctx, account.WorkspaceID)
	if err != nil {
		return botEngineClientContext{}, err
	}
	contextState.SlotsByWorkspace[account.WorkspaceID] = slots
	return contextState, nil
}

func (s *Server) loadBotEngineSlotOptions(ctx context.Context, workspaceID string) ([]botEngineSlotOption, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, nil
	}
	slots, err := s.runtime.repository.AvailableDaySlots(ctx, workspaceID, time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 14))
	if err != nil {
		return nil, err
	}
	options := make([]botEngineSlotOption, 0, min(8, len(slots)))
	for i, slot := range slots {
		if i >= 8 {
			break
		}
		options = append(options, botEngineSlotOption{
			ID:    slot.ID,
			Label: formatTelegramSlotLabel(slot.StartsAt),
		})
	}
	return options, nil
}

func (s *Server) applyBotEngineClientTransition(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, transition botEngineClientTransition) error {
	for _, effect := range transition.Effects {
		if err := s.applyBotEngineClientEffect(ctx, account, inbound, transition.State, effect); err != nil {
			return err
		}
	}
	if err := s.persistBotEngineClientRoute(ctx, account, inbound.chatID, transition.State.Route); err != nil {
		return err
	}
	if err := s.reconcileBotEngineClientBookingSession(ctx, account, inbound.chatID, transition.State); err != nil {
		return err
	}
	return nil
}

func (s *Server) applyBotEngineClientEffect(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, state botEngineClientSession, effect botEngineEffect) error {
	switch effect.Type {
	case "reply":
		return s.enqueueBotEngineReply(ctx, account, inbound.chatID, effect)
	case "configuration.error":
		log.Printf("telegram bot engine configuration error bot=%s account_id=%s message=%q", effect.Bot, account.ID, effect.Message)
		return nil
	case "client.route_selected":
		if strings.TrimSpace(effect.WorkspaceID) == "" {
			return nil
		}
		_, err := s.runtime.repository.EnsureCustomerIdentity(ctx, effect.WorkspaceID, ChannelTelegram, inbound.chatID, inbound.profile)
		return err
	case "crm":
		return s.applyBotEngineClientCRMEffect(ctx, account, inbound, effect)
	case "booking.pending":
		return s.applyBotEngineClientBookingPending(ctx, account, inbound, state, effect)
	case "booking.confirmed":
		return s.applyBotEngineClientBookingConfirmed(ctx, account, inbound, effect)
	case "booking.cancelled":
		return s.applyBotEngineClientBookingCancelled(ctx, account, inbound, effect)
	default:
		return nil
	}
}

func (s *Server) applyBotEngineClientCRMEffect(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, effect botEngineEffect) error {
	targetWorkspaceID := strings.TrimSpace(effect.WorkspaceID)
	if account.AccountScope == ChannelAccountScopeGlobal && targetWorkspaceID == "" {
		return s.promptTelegramMasterPhone(ctx, account, inbound.chatID, false)
	}
	if targetWorkspaceID == "" {
		targetWorkspaceID = account.WorkspaceID
	}
	if strings.TrimSpace(targetWorkspaceID) == "" || strings.TrimSpace(effect.Text) == "" {
		return nil
	}
	result, err := s.receiveTelegramClientInboundEffect(ctx, account, inbound, targetWorkspaceID, effect.Text)
	if err != nil {
		return err
	}
	return s.publishTelegramInboundResult(ctx, result)
}

func (s *Server) applyBotEngineClientBookingPending(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, state botEngineClientSession, effect botEngineEffect) error {
	workspaceID := strings.TrimSpace(effect.WorkspaceID)
	if workspaceID == "" && state.Route.WorkspaceID != "" {
		workspaceID = state.Route.WorkspaceID
	}
	if workspaceID == "" || strings.TrimSpace(effect.SlotID) == "" {
		return nil
	}

	conversation, customer, err := s.ensureTelegramClientConversation(ctx, account, inbound, workspaceID, "Хочу записаться")
	if err != nil {
		return err
	}

	actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}
	booking, err := s.runtime.services.Bookings.CreateBooking(ctx, actor, usecase.CreateBookingInput{
		WorkspaceID:    workspaceID,
		CustomerID:     customer.ID,
		DailySlotID:    effect.SlotID,
		Status:         "pending",
		Notes:          "Создано через TypeScript Telegram client bot",
		ConversationID: conversation.ID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "slot unavailable") {
			return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, conversation.ID, "", TelegramOutboundPayload{
				ChatID: inbound.chatID,
				Text:   "Этот слот уже недоступен. Запросите свободные окна ещё раз.",
				Buttons: []TelegramInlineButton{
					{Text: "Свободные окна", CallbackData: "client:slots"},
				},
			})
		}
		return err
	}
	if _, err := s.runtime.services.BotSessions.StartSession(ctx, actor, usecase.BotSessionInput{
		WorkspaceID: workspaceID,
		BotKind:     string(ChannelKindTelegramClient),
		Scope:       string(BotSessionScopeClient),
		ActorType:   string(BotSessionActorCustomer),
		ActorID:     customer.ID,
		State:       "awaiting_confirm_booking",
		ExpiresAt:   time.Now().UTC().Add(botEngineClientBookingTTL),
	}, map[string]string{
		"bookingId":      booking.ID,
		"conversationId": conversation.ID,
		"dailySlotId":    effect.SlotID,
		"slotLabel":      effect.SlotLabel,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Server) applyBotEngineClientBookingConfirmed(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, effect botEngineEffect) error {
	workspaceID := strings.TrimSpace(effect.WorkspaceID)
	if workspaceID == "" {
		workspaceID = account.WorkspaceID
	}
	if workspaceID == "" {
		return nil
	}

	conversation, customer, err := s.ensureTelegramClientConversation(ctx, account, inbound, workspaceID, "Хочу записаться")
	if err != nil {
		return err
	}

	if effect.Amount > 0 && strings.TrimSpace(effect.CustomerID) != "" {
		actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: workspaceID}
		_, err := s.runtime.services.Bookings.CreateBooking(ctx, actor, usecase.CreateBookingInput{
			WorkspaceID:    workspaceID,
			CustomerID:     effect.CustomerID,
			DailySlotID:    effect.SlotID,
			Amount:         effect.Amount,
			Status:         "confirmed",
			Notes:          "Подтверждено через TypeScript Telegram operator bot",
			ConversationID: effect.ConversationID,
		})
		return err
	}

	botSession, err := s.runtime.repository.LoadBotSession(ctx, workspaceID, BotSessionScopeClient, BotSessionActorCustomer, customer.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	var payload struct {
		BookingID      string `json:"bookingId"`
		ConversationID string `json:"conversationId"`
	}
	if err := json.Unmarshal([]byte(botSession.Payload), &payload); err != nil {
		_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}, workspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
		return nil
	}
	if strings.TrimSpace(payload.BookingID) == "" {
		_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}, workspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
		return nil
	}

	actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}
	if _, err := s.runtime.services.Bookings.ConfirmBooking(ctx, actor, workspaceID, payload.BookingID, 0, firstNonEmptyString(payload.ConversationID, conversation.ID)); err != nil {
		if strings.Contains(err.Error(), "slot unavailable") {
			return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, conversation.ID, "", TelegramOutboundPayload{
				ChatID: inbound.chatID,
				Text:   "Слот уже недоступен. Запросите свободные окна заново.",
				Buttons: []TelegramInlineButton{
					{Text: "Свободные окна", CallbackData: "client:slots"},
				},
			})
		}
		return err
	}
	if err := s.runtime.services.BotSessions.ClearSession(ctx, actor, workspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID); err != nil {
		return err
	}
	s.notifyOperatorsAboutConversation(ctx, conversation, customer)
	return nil
}

func (s *Server) applyBotEngineClientBookingCancelled(ctx context.Context, _ ChannelAccount, inbound telegramClientInbound, effect botEngineEffect) error {
	workspaceID := strings.TrimSpace(effect.WorkspaceID)
	if workspaceID == "" {
		return nil
	}

	conversation, customer, err := s.runtime.repository.ConversationByExternalChat(ctx, workspaceID, ChannelTelegram, inbound.chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	session, err := s.runtime.repository.LoadBotSession(ctx, workspaceID, BotSessionScopeClient, BotSessionActorCustomer, customer.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	var payload struct {
		BookingID string `json:"bookingId"`
	}
	if err := json.Unmarshal([]byte(session.Payload), &payload); err == nil && strings.TrimSpace(payload.BookingID) != "" {
		actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}
		_, _ = s.runtime.services.Bookings.CancelBooking(ctx, actor, workspaceID, payload.BookingID, conversation.ID)
	}
	return s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}, workspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
}

func (s *Server) persistBotEngineClientRoute(ctx context.Context, account ChannelAccount, chatID string, route botEngineClientRouteState) error {
	if account.AccountScope != ChannelAccountScopeGlobal {
		return nil
	}
	state := strings.TrimSpace(route.Kind)
	if state == "" {
		state = "awaiting_master_phone"
	}
	input := usecase.ClientBotRouteInput{
		ChannelAccountID: account.ID,
		ExternalChatID:   chatID,
		State:            state,
		ExpiresAt:        time.Now().UTC().Add(botEngineClientRouteTTL),
	}
	if route.Kind == "ready" {
		input.SelectedWorkspaceID = strings.TrimSpace(route.WorkspaceID)
		input.SelectedMasterPhoneNormalized = strings.TrimSpace(route.MasterPhone)
	}
	_, err := s.runtime.services.BotSessions.StoreClientRoute(ctx, domain.SystemActor(), input)
	return err
}

func (s *Server) reconcileBotEngineClientBookingSession(ctx context.Context, account ChannelAccount, chatID string, state botEngineClientSession) error {
	if state.Booking.Kind == "awaiting_confirmation" {
		return nil
	}

	workspaceID := account.WorkspaceID
	if state.Route.Kind == "ready" && state.Route.WorkspaceID != "" {
		workspaceID = state.Route.WorkspaceID
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	_, customer, err := s.runtime.repository.ConversationByExternalChat(ctx, workspaceID, ChannelTelegram, chatID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	return s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}, workspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
}

func (s *Server) receiveTelegramClientInboundEffect(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, workspaceID, text string) (usecase.InboundResult, error) {
	input := inboundUsecaseInput(
		ChannelTelegram,
		account.ID,
		inbound.chatID,
		inbound.externalMessageID,
		text,
		inbound.timestamp,
		inbound.profile,
	)
	return s.runtime.services.Inbox.ReceiveInboundMessageForWorkspace(ctx, workspaceID, input)
}

func (s *Server) publishTelegramInboundResult(ctx context.Context, result usecase.InboundResult) error {
	if !result.Stored {
		return nil
	}
	conversation, _, customer, err := s.runtime.repository.ConversationDetail(ctx, result.WorkspaceID, result.ConversationID)
	if err != nil {
		return nil
	}
	_ = s.publishEvent(ctx, SSEEvent{Type: EventMessageNew, Data: Message{ID: result.MessageID, ConversationID: result.ConversationID}})
	_ = s.publishDashboard(ctx, result.WorkspaceID)
	s.notifyOperatorsAboutConversation(ctx, conversation, customer)
	return nil
}

func (s *Server) ensureTelegramClientConversation(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, workspaceID, bootstrapText string) (Conversation, Customer, error) {
	conversation, customer, err := s.runtime.repository.ConversationByExternalChat(ctx, workspaceID, ChannelTelegram, inbound.chatID)
	if err == nil {
		return conversation, customer, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, Customer{}, err
	}
	if strings.TrimSpace(bootstrapText) == "" {
		bootstrapText = "Хочу записаться"
	}
	result, receiveErr := s.receiveTelegramClientInboundEffect(ctx, account, inbound, workspaceID, bootstrapText)
	if receiveErr != nil {
		return Conversation{}, Customer{}, receiveErr
	}
	_ = s.publishTelegramInboundResult(ctx, result)
	return s.runtime.repository.ConversationByExternalChat(ctx, workspaceID, ChannelTelegram, inbound.chatID)
}

func (s *Server) enqueueBotEngineReply(ctx context.Context, account ChannelAccount, chatID string, effect botEngineEffect) error {
	kind := OutboundKindTelegramSendText
	payload := TelegramOutboundPayload{
		ChatID: chatID,
		Text:   effect.Text,
	}
	if len(effect.Buttons) > 0 {
		kind = OutboundKindTelegramSendInline
		payload.Buttons = make([]TelegramInlineButton, 0, len(effect.Buttons))
		for _, button := range effect.Buttons {
			if strings.TrimSpace(button.Text) == "" || strings.TrimSpace(button.Action) == "" {
				continue
			}
			payload.Buttons = append(payload.Buttons, TelegramInlineButton{
				Text:         button.Text,
				CallbackData: button.Action,
			})
		}
	}
	return s.enqueueTelegramOutbound(ctx, account, kind, "", "", payload)
}

func (s *Server) buildBotEngineOperatorContext(ctx context.Context, binding OperatorBotBinding) (botEngineOperatorContext, *botEngineOperatorWorkspace, error) {
	contextState := botEngineOperatorContext{
		LinkBindings: []botEngineOperatorLinkBinding{},
		Workspaces:   []botEngineOperatorWorkspace{},
	}
	if strings.TrimSpace(binding.WorkspaceID) == "" {
		return contextState, nil, nil
	}

	workspace, err := s.runtime.repository.WorkspaceByID(ctx, binding.WorkspaceID)
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}
	dashboard, err := s.runtime.repository.Dashboard(ctx, binding.WorkspaceID)
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}
	bookings, err := s.runtime.repository.Bookings(ctx, binding.WorkspaceID, "all")
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}
	availableSlots, err := s.runtime.repository.AvailableDaySlots(ctx, binding.WorkspaceID, time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 14))
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}
	weekSlots, err := s.runtime.repository.WeekSlots(ctx, binding.WorkspaceID, time.Now().UTC())
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}
	operatorSettings, err := s.runtime.repository.OperatorBotSettings(ctx, binding.WorkspaceID, binding.UserID, s.cfg.OperatorBotUsername, s.cfg.PublicBaseURL)
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}
	botConfig, faqItems, err := s.runtime.repository.BotConfig(ctx, binding.WorkspaceID)
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}
	conversations, err := s.runtime.repository.Conversations(ctx, binding.WorkspaceID)
	if err != nil {
		return botEngineOperatorContext{}, nil, err
	}

	slotOptions := make([]botEngineSlotOption, 0, min(8, len(availableSlots)))
	for i, slot := range availableSlots {
		if i >= 8 {
			break
		}
		slotOptions = append(slotOptions, botEngineSlotOption{
			ID:    slot.ID,
			Label: formatTelegramSlotLabel(slot.StartsAt),
		})
	}

	revenue := 0
	nextSlot := "нет"
	for _, booking := range bookings {
		if booking.Status == BookingCancelled {
			continue
		}
		if booking.Amount > 0 {
			revenue += booking.Amount
		}
		if nextSlot == "нет" && booking.StartsAt.After(time.Now().UTC()) {
			nextSlot = booking.StartsAt.In(time.Local).Format("Mon 02.01 15:04")
		}
	}
	if nextSlot == "нет" && len(availableSlots) > 0 {
		nextSlot = availableSlots[0].StartsAt.In(time.Local).Format("Mon 02.01 15:04")
	}

	operatorConversations := make([]botEngineOperatorConversation, 0, len(conversations))
	for _, conversation := range conversations {
		item := botEngineOperatorConversation{
			ID:              conversation.ID,
			Title:           conversation.Title,
			Provider:        string(conversation.Provider),
			CustomerName:    conversation.Title,
			CustomerID:      conversation.CustomerID,
			Status:          string(conversation.Status),
			LastMessageText: conversation.LastMessageText,
			UnreadCount:     conversation.UnreadCount,
			SlotOptions:     append([]botEngineSlotOption(nil), slotOptions...),
		}
		customer, customerErr := s.runtime.repository.Customer(ctx, binding.WorkspaceID, conversation.CustomerID)
		if customerErr == nil {
			item.CustomerName = firstNonEmptyString(customer.Name, item.CustomerName)
			item.CustomerPhone = customer.Phone
		}
		operatorConversations = append(operatorConversations, item)
	}

	workspaceState := &botEngineOperatorWorkspace{
		ID:   workspace.ID,
		Name: workspace.Name,
		Dashboard: botEngineOperatorDashboard{
			TodayBookings: dashboard.TodayBookings,
			NewMessages:   dashboard.NewMessages,
			Revenue:       revenue,
			FreeSlots:     len(availableSlots),
			NextSlot:      nextSlot,
		},
		Conversations: operatorConversations,
		WeekSlots:     make([]botEngineOperatorWeekSlot, 0, len(weekSlots)),
		Settings: botEngineOperatorSettings{
			AutoReply:         botConfig.AutoReply,
			HandoffEnabled:    botConfig.HandoffEnabled,
			TelegramChatLabel: binding.TelegramChatID,
			WebhookURL:        operatorSettings.OperatorWebhookURL,
			FAQQuestions:      make([]string, 0, len(faqItems)),
		},
	}
	if operatorSettings.Binding != nil && strings.TrimSpace(operatorSettings.Binding.TelegramChatID) != "" {
		workspaceState.Settings.TelegramChatLabel = operatorSettings.Binding.TelegramChatID
	}
	for _, day := range weekSlots {
		workspaceState.WeekSlots = append(workspaceState.WeekSlots, botEngineOperatorWeekSlot{
			Label:     day.Label,
			SlotCount: len(day.Slots),
		})
	}
	for _, item := range faqItems {
		if strings.TrimSpace(item.Question) != "" {
			workspaceState.Settings.FAQQuestions = append(workspaceState.Settings.FAQQuestions, item.Question)
		}
	}
	contextState.Workspaces = append(contextState.Workspaces, *workspaceState)
	return contextState, workspaceState, nil
}

func (s *Server) loadBotEngineOperatorSession(ctx context.Context, binding OperatorBotBinding, workspaceState *botEngineOperatorWorkspace) (*botEngineOperatorSession, error) {
	session := &botEngineOperatorSession{
		Binding:     botEngineOperatorBindingState{Kind: "unbound"},
		Interaction: botEngineOperatorInteractionState{Kind: "idle"},
	}
	if strings.TrimSpace(binding.WorkspaceID) == "" {
		return session, nil
	}
	session.Binding = botEngineOperatorBindingState{
		Kind:        "bound",
		WorkspaceID: binding.WorkspaceID,
		UserID:      binding.UserID,
		ChatID:      binding.TelegramChatID,
	}

	stored, err := s.runtime.repository.LoadBotSession(ctx, binding.WorkspaceID, BotSessionScopeOperator, BotSessionActorUser, binding.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session, nil
		}
		return nil, err
	}

	switch stored.State {
	case "awaiting_operator_reply":
		var payload struct {
			ConversationID string `json:"conversationId"`
		}
		if json.Unmarshal([]byte(stored.Payload), &payload) == nil && strings.TrimSpace(payload.ConversationID) != "" {
			session.Interaction = botEngineOperatorInteractionState{
				Kind:           "awaiting_reply",
				ConversationID: payload.ConversationID,
			}
		}
	case "awaiting_price":
		var payload struct {
			ConversationID string `json:"conversationId"`
			CustomerID     string `json:"customerId"`
			DailySlotID    string `json:"dailySlotId"`
		}
		if json.Unmarshal([]byte(stored.Payload), &payload) == nil && strings.TrimSpace(payload.ConversationID) != "" && strings.TrimSpace(payload.DailySlotID) != "" {
			session.Interaction = botEngineOperatorInteractionState{
				Kind:           "awaiting_price",
				ConversationID: payload.ConversationID,
				CustomerID:     payload.CustomerID,
				SlotID:         payload.DailySlotID,
				SlotLabel:      findBotEngineConversationSlotLabel(workspaceState, payload.ConversationID, payload.DailySlotID),
			}
		}
	}
	if session.Interaction.SlotLabel == "" {
		session.Interaction.SlotLabel = session.Interaction.SlotID
	}
	return session, nil
}

func (s *Server) applyBotEngineOperatorTransition(ctx context.Context, account ChannelAccount, chatID string, binding OperatorBotBinding, transition botEngineOperatorTransition) error {
	for _, effect := range transition.Effects {
		if err := s.applyBotEngineOperatorEffect(ctx, account, chatID, binding, effect); err != nil {
			return err
		}
	}
	return s.persistBotEngineOperatorSession(ctx, binding, transition.State)
}

func (s *Server) applyBotEngineOperatorEffect(ctx context.Context, account ChannelAccount, chatID string, binding OperatorBotBinding, effect botEngineEffect) error {
	switch effect.Type {
	case "reply":
		return s.enqueueBotEngineReply(ctx, account, chatID, effect)
	case "configuration.error":
		log.Printf("telegram bot engine configuration error bot=%s workspace_id=%s message=%q", effect.Bot, effect.WorkspaceID, effect.Message)
		return nil
	case "crm":
		return s.applyBotEngineOperatorCRMEffect(ctx, binding, effect)
	case "settings.auto_reply_changed":
		if strings.TrimSpace(binding.WorkspaceID) == "" {
			return nil
		}
		actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: binding.WorkspaceID, UserID: binding.UserID}
		return s.runtime.services.BotSettings.ToggleAutoReply(ctx, actor, binding.WorkspaceID, effect.Enabled)
	default:
		return nil
	}
}

func (s *Server) applyBotEngineOperatorCRMEffect(ctx context.Context, binding OperatorBotBinding, effect botEngineEffect) error {
	if strings.TrimSpace(binding.WorkspaceID) == "" {
		return nil
	}
	actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: binding.WorkspaceID, UserID: binding.UserID}
	switch effect.Intent {
	case "operator.reply_sent":
		_, err := s.runtime.services.Dialogs.ReplyToDialog(ctx, actor, effect.ConversationID, effect.Text)
		return err
	case "operator.take_dialog":
		return s.runtime.services.Dialogs.TakeDialogByHuman(ctx, actor, effect.ConversationID)
	case "operator.return_to_auto":
		return s.runtime.services.Dialogs.ReturnDialogToAuto(ctx, actor, effect.ConversationID)
	case "operator.close_dialog":
		return s.runtime.services.Dialogs.CloseDialog(ctx, actor, effect.ConversationID)
	default:
		return nil
	}
}

func (s *Server) persistBotEngineOperatorSession(ctx context.Context, binding OperatorBotBinding, session botEngineOperatorSession) error {
	if strings.TrimSpace(binding.WorkspaceID) == "" {
		return nil
	}
	actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: binding.WorkspaceID, UserID: binding.UserID}
	switch session.Interaction.Kind {
	case "awaiting_reply":
		_, err := s.runtime.services.BotSessions.StartSession(ctx, actor, usecase.BotSessionInput{
			WorkspaceID: binding.WorkspaceID,
			BotKind:     string(ChannelKindTelegramOperator),
			Scope:       string(BotSessionScopeOperator),
			ActorType:   string(BotSessionActorUser),
			ActorID:     binding.UserID,
			State:       "awaiting_operator_reply",
			ExpiresAt:   time.Now().UTC().Add(botEngineOperatorSessionTTL),
		}, map[string]string{
			"conversationId": session.Interaction.ConversationID,
		})
		return err
	case "awaiting_price":
		payload := map[string]string{
			"conversationId": session.Interaction.ConversationID,
			"dailySlotId":    session.Interaction.SlotID,
		}
		if strings.TrimSpace(session.Interaction.CustomerID) != "" {
			payload["customerId"] = session.Interaction.CustomerID
		}
		_, err := s.runtime.services.BotSessions.StartSession(ctx, actor, usecase.BotSessionInput{
			WorkspaceID: binding.WorkspaceID,
			BotKind:     string(ChannelKindTelegramOperator),
			Scope:       string(BotSessionScopeOperator),
			ActorType:   string(BotSessionActorUser),
			ActorID:     binding.UserID,
			State:       "awaiting_price",
			ExpiresAt:   time.Now().UTC().Add(botEngineOperatorSessionTTL),
		}, payload)
		return err
	default:
		return s.runtime.services.BotSessions.ClearSession(ctx, actor, binding.WorkspaceID, string(BotSessionScopeOperator), string(BotSessionActorUser), binding.UserID)
	}
}

func findBotEngineConversationSlotLabel(workspace *botEngineOperatorWorkspace, conversationID, slotID string) string {
	if workspace == nil || strings.TrimSpace(conversationID) == "" || strings.TrimSpace(slotID) == "" {
		return ""
	}
	for _, conversation := range workspace.Conversations {
		if conversation.ID != conversationID {
			continue
		}
		for _, slot := range conversation.SlotOptions {
			if slot.ID == slotID {
				return slot.Label
			}
		}
	}
	return ""
}

func formatTelegramSlotLabel(startsAt time.Time) string {
	if startsAt.IsZero() {
		return ""
	}
	return startsAt.In(time.Local).Format("02.01 15:04")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
