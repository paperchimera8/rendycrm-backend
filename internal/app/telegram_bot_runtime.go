package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
	tgapi "github.com/vital/rendycrm-app/internal/telegram"
	"github.com/vital/rendycrm-app/internal/usecase"
)

const (
	botRuntimeTokenHeader           = "X-Bot-Runtime-Token"
	botRuntimeHTTPTimeout           = 10 * time.Second
	botRuntimeHTTPBodyLimitBytes    = 1 << 20
	botRuntimeClientRouteTTL        = 30 * 24 * time.Hour
	botRuntimeClientBookingTTL      = 30 * time.Minute
	botRuntimeOperatorSessionTTL    = 15 * time.Minute
	telegramCallbackActionCooldown  = 10 * time.Second
	telegramInboundDeliveryCooldown = 2 * time.Minute
	telegramOperatorCommandCooldown = 30 * time.Second
	telegramOperatorReplyCooldown   = 30 * time.Second
	telegramPromptCooldown          = 45 * time.Second
)

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
	BookingID      string            `json:"bookingId,omitempty"`
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
	Code           string            `json:"code,omitempty"`
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

type botEngineOperatorReminder struct {
	BookingID     string `json:"bookingId"`
	CustomerName  string `json:"customerName"`
	StartsAtLabel string `json:"startsAtLabel"`
	Enabled       bool   `json:"enabled"`
	Sent          bool   `json:"sent"`
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
	Reminders     []botEngineOperatorReminder     `json:"reminders"`
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

type botEngineOperatorTransition struct {
	State   botEngineOperatorSession `json:"state"`
	Effects []botEngineEffect        `json:"effects"`
}

type botRuntimeClientPrepareRequest struct {
	AccountID string          `json:"accountId"`
	Secret    string          `json:"secret"`
	Update    json.RawMessage `json:"update"`
}

type botRuntimeClientPrepareResponse struct {
	Skip     bool                    `json:"skip"`
	Session  *botEngineClientSession `json:"session,omitempty"`
	Event    *botEngineClientEvent   `json:"event,omitempty"`
	Context  *botEngineClientContext `json:"context,omitempty"`
	Snapshot *botRuntimeClientSnap   `json:"snapshot,omitempty"`
}

type botRuntimeClientSnap struct {
	AccountID         string         `json:"accountId"`
	UpdateID          int64          `json:"updateId"`
	ChatID            string         `json:"chatId"`
	MessageID         int64          `json:"messageId,omitempty"`
	CallbackID        string         `json:"callbackId,omitempty"`
	CallbackData      string         `json:"callbackData,omitempty"`
	ExternalMessageID string         `json:"externalMessageId,omitempty"`
	Timestamp         time.Time      `json:"timestamp"`
	Profile           InboundProfile `json:"profile"`
}

type botRuntimeClientApplyRequest struct {
	Snapshot   botRuntimeClientSnap      `json:"snapshot"`
	Transition botEngineClientTransition `json:"transition"`
}

type botRuntimeOperatorPrepareRequest struct {
	Secret string          `json:"secret"`
	Update json.RawMessage `json:"update"`
}

type botRuntimeOperatorPrepareResponse struct {
	Skip     bool                      `json:"skip"`
	Session  *botEngineOperatorSession `json:"session,omitempty"`
	Event    *botEngineOperatorEvent   `json:"event,omitempty"`
	Context  *botEngineOperatorContext `json:"context,omitempty"`
	Snapshot *botRuntimeOperatorSnap   `json:"snapshot,omitempty"`
}

type botRuntimeOperatorSnap struct {
	AccountID      string `json:"accountId"`
	UpdateID       int64  `json:"updateId"`
	ChatID         string `json:"chatId"`
	TelegramUserID string `json:"telegramUserId"`
	MessageID      int64  `json:"messageId,omitempty"`
	CallbackID     string `json:"callbackId,omitempty"`
	CallbackData   string `json:"callbackData,omitempty"`
	Command        string `json:"command,omitempty"`
}

type botEngineOperatorContextScope struct {
	Dashboard     bool
	Conversations bool
	WeekSlots     bool
	Reminders     bool
	Settings      bool
	FAQ           bool
}

type botRuntimeOperatorApplyRequest struct {
	Snapshot   botRuntimeOperatorSnap      `json:"snapshot"`
	Transition botEngineOperatorTransition `json:"transition"`
}

type botRuntimeApplyResponse struct {
	OK        bool `json:"ok"`
	Duplicate bool `json:"duplicate,omitempty"`
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

func (s *Server) proxyBotRuntimeWebhook(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.cfg.BotRuntimeBaseURL) == "" {
		s.writeError(w, http.StatusServiceUnavailable, "bot runtime is not configured")
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, botRuntimeHTTPBodyLimitBytes))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid webhook payload")
		return
	}

	requestURL := s.cfg.BotRuntimeBaseURL + r.URL.Path
	if r.URL.RawQuery != "" {
		requestURL += "?" + r.URL.RawQuery
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, requestURL, bytes.NewReader(body))
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to proxy webhook")
		return
	}
	request.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if secret := strings.TrimSpace(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")); secret != "" {
		request.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	}

	client := &http.Client{Timeout: botRuntimeHTTPTimeout}
	response, err := client.Do(request)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "bot runtime is unavailable")
		return
	}
	defer response.Body.Close()

	if contentType := strings.TrimSpace(response.Header.Get("Content-Type")); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, botRuntimeHTTPBodyLimitBytes))
}

func (s *Server) handleBotRuntimeClientPrepare(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeBotRuntimeRequest(w, r) {
		return
	}
	var request botRuntimeClientPrepareRequest
	if err := s.decodeJSON(r, &request); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	response, err := s.prepareTelegramClientRuntime(r.Context(), request)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		log.Printf("bot runtime client prepare: %v", err)
		s.writeError(w, status, "internal server error")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleBotRuntimeClientApply(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeBotRuntimeRequest(w, r) {
		return
	}
	var request botRuntimeClientApplyRequest
	if err := s.decodeJSON(r, &request); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if strings.TrimSpace(request.Snapshot.AccountID) == "" || strings.TrimSpace(request.Snapshot.ChatID) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid snapshot")
		return
	}
	duplicate, err := s.applyTelegramClientRuntime(r.Context(), request)
	if err != nil {
		log.Printf("bot runtime client apply: %v", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	s.writeJSON(w, http.StatusOK, botRuntimeApplyResponse{OK: true, Duplicate: duplicate})
}

func (s *Server) handleBotRuntimeOperatorPrepare(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeBotRuntimeRequest(w, r) {
		return
	}
	var request botRuntimeOperatorPrepareRequest
	if err := s.decodeJSON(r, &request); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	response, err := s.prepareTelegramOperatorRuntime(r.Context(), request)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		log.Printf("bot runtime operator prepare: %v", err)
		s.writeError(w, status, "internal server error")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleBotRuntimeOperatorApply(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeBotRuntimeRequest(w, r) {
		return
	}
	var request botRuntimeOperatorApplyRequest
	if err := s.decodeJSON(r, &request); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if strings.TrimSpace(request.Snapshot.AccountID) == "" || strings.TrimSpace(request.Snapshot.ChatID) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid snapshot")
		return
	}
	duplicate, err := s.applyTelegramOperatorRuntime(r.Context(), request)
	if err != nil {
		log.Printf("bot runtime operator apply: %v", err)
		s.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	s.writeJSON(w, http.StatusOK, botRuntimeApplyResponse{OK: true, Duplicate: duplicate})
}

func (s *Server) authorizeBotRuntimeRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	if strings.TrimSpace(s.cfg.BotRuntimeToken) == "" {
		s.writeError(w, http.StatusServiceUnavailable, "bot runtime token is not configured")
		return false
	}
	provided := strings.TrimSpace(r.Header.Get(botRuntimeTokenHeader))
	if subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.BotRuntimeToken)) != 1 {
		s.writeError(w, http.StatusUnauthorized, "invalid bot runtime token")
		return false
	}
	return true
}

func (s *Server) prepareTelegramClientRuntime(ctx context.Context, request botRuntimeClientPrepareRequest) (botRuntimeClientPrepareResponse, error) {
	if strings.TrimSpace(request.AccountID) == "" {
		return botRuntimeClientPrepareResponse{}, errors.New("telegram client account id is required")
	}
	account, err := s.runtime.repository.ChannelAccountByID(ctx, request.AccountID)
	if err != nil {
		return botRuntimeClientPrepareResponse{}, err
	}
	if account.Provider != ChannelTelegram || account.ChannelKind != ChannelKindTelegramClient {
		return botRuntimeClientPrepareResponse{}, errors.New("channel account is not telegram client")
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(request.Secret)), []byte(strings.TrimSpace(account.WebhookSecret))) != 1 {
		return botRuntimeClientPrepareResponse{}, errors.New("invalid telegram client webhook secret")
	}

	update, err := decodeTelegramUpdate(request.Update)
	if err != nil {
		return botRuntimeClientPrepareResponse{}, errors.New("invalid telegram update")
	}
	inbound, err := buildTelegramClientInbound(update)
	if err != nil {
		return botRuntimeClientPrepareResponse{Skip: true}, nil
	}
	if inbound.callbackWithoutMessage {
		s.answerTelegramCallback(ctx, account, inbound.callbackID, "")
		return botRuntimeClientPrepareResponse{Skip: true}, nil
	}
	if strings.TrimSpace(inbound.chatID) == "" {
		return botRuntimeClientPrepareResponse{Skip: true}, nil
	}
	if inbound.callbackID != "" {
		s.answerTelegramCallback(ctx, account, inbound.callbackID, "")
	}

	session, err := s.loadBotEngineClientSession(ctx, account, inbound.chatID)
	if err != nil {
		return botRuntimeClientPrepareResponse{}, err
	}
	contextState, err := s.buildBotEngineClientContext(ctx, account, session)
	if err != nil {
		return botRuntimeClientPrepareResponse{}, err
	}
	event := buildBotEngineClientEvent(update, inbound)
	event.EventID = botRuntimeEventIDFromUpdate(update)

	return botRuntimeClientPrepareResponse{
		Session: session,
		Event:   &event,
		Context: &contextState,
		Snapshot: &botRuntimeClientSnap{
			AccountID:         account.ID,
			UpdateID:          update.UpdateID,
			ChatID:            inbound.chatID,
			MessageID:         inbound.messageID,
			CallbackID:        inbound.callbackID,
			CallbackData:      inbound.callbackData,
			ExternalMessageID: inbound.externalMessageID,
			Timestamp:         inbound.timestamp,
			Profile:           inbound.profile,
		},
	}, nil
}

func (s *Server) applyTelegramClientRuntime(ctx context.Context, request botRuntimeClientApplyRequest) (bool, error) {
	account, err := s.runtime.repository.ChannelAccountByID(ctx, request.Snapshot.AccountID)
	if err != nil {
		return false, err
	}
	freshDelivery, err := s.claimTelegramInboundDelivery(ctx, account.ID, ChannelKindTelegramClient, request.Snapshot.ChatID, request.Snapshot.MessageID, request.Snapshot.CallbackID)
	if err != nil {
		return false, err
	}
	if !freshDelivery {
		return true, nil
	}
	isNew, err := s.runtime.repository.MarkTelegramUpdateProcessed(ctx, account.WorkspaceID, account.ID, ChannelKindTelegramClient, request.Snapshot.UpdateID, request.Snapshot.ChatID, request.Snapshot.MessageID, request.Snapshot.CallbackID)
	if err != nil {
		return false, err
	}
	if !isNew {
		return true, nil
	}
	if request.Snapshot.CallbackData != "" {
		freshAction, err := s.claimTelegramCallbackAction(ctx, account.ID, ChannelKindTelegramClient, request.Snapshot.ChatID, request.Snapshot.MessageID, request.Snapshot.CallbackData)
		if err != nil {
			return false, err
		}
		if !freshAction {
			return true, nil
		}
	}

	inbound := telegramClientInbound{
		chatID:            request.Snapshot.ChatID,
		messageID:         request.Snapshot.MessageID,
		callbackID:        request.Snapshot.CallbackID,
		callbackData:      request.Snapshot.CallbackData,
		externalMessageID: request.Snapshot.ExternalMessageID,
		timestamp:         request.Snapshot.Timestamp,
		profile:           request.Snapshot.Profile,
	}
	if err := s.applyBotEngineClientTransition(ctx, account, inbound, request.Transition); err != nil {
		return false, err
	}
	return false, nil
}

func (s *Server) prepareTelegramOperatorRuntime(ctx context.Context, request botRuntimeOperatorPrepareRequest) (botRuntimeOperatorPrepareResponse, error) {
	if strings.TrimSpace(request.Secret) == "" {
		return botRuntimeOperatorPrepareResponse{}, errors.New("telegram operator webhook secret is required")
	}
	account, err := s.runtime.repository.ChannelAccountByWebhookSecret(ctx, ChannelKindTelegramOperator, request.Secret)
	if err != nil {
		return botRuntimeOperatorPrepareResponse{}, errors.New("invalid telegram operator webhook secret")
	}
	update, err := decodeTelegramUpdate(request.Update)
	if err != nil {
		return botRuntimeOperatorPrepareResponse{}, errors.New("invalid telegram update")
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message == nil {
		s.answerTelegramCallback(ctx, account, callbackIDFromUpdate(update), "")
		return botRuntimeOperatorPrepareResponse{Skip: true}, nil
	}

	chatID, userID, text, err := parseOperatorUpdate(update)
	if err != nil {
		return botRuntimeOperatorPrepareResponse{Skip: true}, nil
	}
	log.Printf(
		"telegram inbound bot=operator stage=prepare_start update_id=%d chat_id=%s message_id=%d callback_id=%q value=%q",
		update.UpdateID,
		chatID,
		messageIDFromUpdate(update),
		callbackIDFromUpdate(update),
		telegramLogValue(text),
	)
	if callbackID := callbackIDFromUpdate(update); callbackID != "" {
		s.answerTelegramCallback(ctx, account, callbackID, "")
	}
	event := buildBotEngineOperatorEvent(update, text)
	scope := operatorContextScopeForEvent(event)

	binding, err := s.runtime.repository.ActiveOperatorBindingByTelegramChat(ctx, chatID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return botRuntimeOperatorPrepareResponse{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		binding = OperatorBotBinding{}
	}

	linkBindings := make([]botEngineOperatorLinkBinding, 0, 1)
	if code := operatorLinkCodeFromInput(text, strings.TrimSpace(binding.WorkspaceID) == ""); code != "" {
		linkCode, lookupErr := s.runtime.repository.OperatorLinkCodeByCode(ctx, code)
		if lookupErr == nil {
			linkBindings = append(linkBindings, botEngineOperatorLinkBinding{
				Code:        linkCode.Code,
				WorkspaceID: linkCode.WorkspaceID,
				UserID:      linkCode.UserID,
				ChatID:      chatID,
			})
		} else if !errors.Is(lookupErr, sql.ErrNoRows) {
			return botRuntimeOperatorPrepareResponse{}, lookupErr
		}
	}

	contextState, workspaceState, err := s.buildBotEngineOperatorContext(ctx, binding, linkBindings, scope)
	if err != nil {
		return botRuntimeOperatorPrepareResponse{}, err
	}
	session, err := s.loadBotEngineOperatorSession(ctx, binding, workspaceState)
	if err != nil {
		return botRuntimeOperatorPrepareResponse{}, err
	}

	if event.Type == "message" && strings.TrimSpace(event.Text) == "" {
		return botRuntimeOperatorPrepareResponse{Skip: true}, nil
	}
	event.EventID = botRuntimeEventIDFromUpdate(update)
	log.Printf(
		"telegram inbound bot=operator stage=engine_prepare update_id=%d chat_id=%s message_id=%d callback_id=%q value=%q",
		update.UpdateID,
		chatID,
		messageIDFromUpdate(update),
		callbackIDFromUpdate(update),
		telegramLogValue(text),
	)

	return botRuntimeOperatorPrepareResponse{
		Session: session,
		Event:   &event,
		Context: &contextState,
		Snapshot: &botRuntimeOperatorSnap{
			AccountID:      account.ID,
			UpdateID:       update.UpdateID,
			ChatID:         chatID,
			TelegramUserID: userID,
			MessageID:      messageIDFromUpdate(update),
			CallbackID:     callbackIDFromUpdate(update),
			CallbackData:   normalizeOperatorCommand(callbackDataFromUpdate(update)),
			Command:        telegramOperatorCommandForUpdate(update, text),
		},
	}, nil
}

func (s *Server) applyTelegramOperatorRuntime(ctx context.Context, request botRuntimeOperatorApplyRequest) (bool, error) {
	account, err := s.runtime.repository.ChannelAccountByID(ctx, request.Snapshot.AccountID)
	if err != nil {
		return false, err
	}
	freshDelivery, err := s.claimTelegramInboundDelivery(ctx, account.ID, ChannelKindTelegramOperator, request.Snapshot.ChatID, request.Snapshot.MessageID, request.Snapshot.CallbackID)
	if err != nil {
		return false, err
	}
	if !freshDelivery {
		return true, nil
	}
	isNew, err := s.runtime.repository.MarkTelegramUpdateProcessed(ctx, account.WorkspaceID, account.ID, ChannelKindTelegramOperator, request.Snapshot.UpdateID, request.Snapshot.ChatID, request.Snapshot.MessageID, request.Snapshot.CallbackID)
	if err != nil {
		return false, err
	}
	if !isNew {
		return true, nil
	}
	if request.Snapshot.CallbackData != "" {
		freshAction, err := s.claimTelegramCallbackAction(ctx, account.ID, ChannelKindTelegramOperator, request.Snapshot.ChatID, request.Snapshot.MessageID, request.Snapshot.CallbackData)
		if err != nil {
			return false, err
		}
		if !freshAction {
			return true, nil
		}
	}
	if err := s.applyBotEngineOperatorTransition(ctx, account, request.Snapshot, request.Transition); err != nil {
		return false, err
	}
	return false, nil
}

func decodeTelegramUpdate(payload json.RawMessage) (tgapi.Update, error) {
	var update tgapi.Update
	if len(payload) == 0 {
		return tgapi.Update{}, errors.New("empty telegram update")
	}
	if err := json.Unmarshal(payload, &update); err != nil {
		return tgapi.Update{}, err
	}
	return update, nil
}

func buildTelegramClientInbound(update tgapi.Update) (telegramClientInbound, error) {
	inbound := telegramClientInbound{timestamp: time.Now().UTC()}
	switch {
	case update.Message != nil:
		inbound.chatID = strconv.FormatInt(update.Message.Chat.ID, 10)
		inbound.messageID = update.Message.MessageID
		inbound.text = strings.TrimSpace(update.Message.Text)
		inbound.timestamp = time.Unix(update.Message.Date, 0).UTC()
		if update.Message.From != nil {
			inbound.profile.Username = usernameFromUser(update.Message.From)
			inbound.profile.Name = strings.TrimSpace(update.Message.From.FirstName + " " + update.Message.From.LastName)
		}
		inbound.profile.Phone = phoneFromContact(update.Message.Contact)
		inbound.externalMessageID = strconv.FormatInt(update.Message.MessageID, 10)
		return inbound, nil
	case update.CallbackQuery != nil:
		inbound.callbackID = update.CallbackQuery.ID
		inbound.callbackData = strings.TrimSpace(update.CallbackQuery.Data)
		inbound.externalMessageID = telegramCallbackExternalMessageID(update)
		inbound.profile.Username = usernameFromUser(&update.CallbackQuery.From)
		inbound.profile.Name = strings.TrimSpace(update.CallbackQuery.From.FirstName + " " + update.CallbackQuery.From.LastName)
		if update.CallbackQuery.Message == nil {
			inbound.callbackWithoutMessage = true
			return inbound, nil
		}
		inbound.chatID = strconv.FormatInt(update.CallbackQuery.Message.Chat.ID, 10)
		inbound.messageID = update.CallbackQuery.Message.MessageID
		return inbound, nil
	default:
		return inbound, errors.New("empty telegram update")
	}
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

func buildBotEngineOperatorEvent(update tgapi.Update, text string) botEngineOperatorEvent {
	if payload, isStart := telegramStartPayload(text); isStart {
		return botEngineOperatorEvent{
			Type:    "start",
			Payload: strings.TrimSpace(payload),
		}
	}
	if update.CallbackQuery != nil {
		return botEngineOperatorEvent{
			Type: "callback",
			Data: normalizeOperatorCommand(update.CallbackQuery.Data),
		}
	}
	return botEngineOperatorEvent{
		Type: "message",
		Text: normalizeOperatorCommand(text),
	}
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
				if workspaceErr == nil {
					session.Route = botEngineClientRouteState{
						Kind:          "ready",
						WorkspaceID:   route.SelectedWorkspaceID,
						WorkspaceName: workspace.Name,
						MasterPhone:   firstNonEmptyString(route.SelectedMasterPhoneNormalized, workspace.MasterPhoneNormalized),
					}
				} else if !errors.Is(workspaceErr, sql.ErrNoRows) {
					return nil, workspaceErr
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
	slots, err := s.runtime.repository.FreeDaySlotsBetween(ctx, workspaceID, time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 14))
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
	return s.reconcileBotEngineClientBookingSession(ctx, account, inbound.chatID, transition.State)
}

func (s *Server) applyBotEngineClientEffect(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, state botEngineClientSession, effect botEngineEffect) error {
	switch effect.Type {
	case "reply":
		return s.enqueueBotEngineReply(ctx, account, inbound.chatID, &state, effect, inbound.messageID, inbound.callbackID, inbound.callbackData)
	case "configuration.error":
		log.Printf("telegram bot runtime configuration error bot=%s account_id=%s message=%q", effect.Bot, account.ID, effect.Message)
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
		return s.applyBotEngineClientBookingCancelled(ctx, inbound, effect)
	default:
		return nil
	}
}

func (s *Server) applyBotEngineClientCRMEffect(ctx context.Context, account ChannelAccount, inbound telegramClientInbound, effect botEngineEffect) error {
	targetWorkspaceID := strings.TrimSpace(effect.WorkspaceID)
	if account.AccountScope == ChannelAccountScopeGlobal && targetWorkspaceID == "" {
		return s.enqueueTelegramMasterPhonePrompt(ctx, account, inbound.chatID, false, inbound.messageID, inbound.callbackID, inbound.callbackData)
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
			return s.enqueueTelegramCalendarReply(ctx, account, inbound.chatID, workspaceID, conversation.ID, "Этот слот уже недоступен. Откройте календарь и выберите другое время.", inbound.messageID, inbound.callbackID, inbound.callbackData)
		}
		return err
	}
	_, err = s.runtime.services.BotSessions.StartSession(ctx, actor, usecase.BotSessionInput{
		WorkspaceID: workspaceID,
		BotKind:     string(ChannelKindTelegramClient),
		Scope:       string(BotSessionScopeClient),
		ActorType:   string(BotSessionActorCustomer),
		ActorID:     customer.ID,
		State:       "awaiting_confirm_booking",
		ExpiresAt:   time.Now().UTC().Add(botRuntimeClientBookingTTL),
	}, map[string]string{
		"bookingId":      booking.ID,
		"conversationId": conversation.ID,
		"dailySlotId":    effect.SlotID,
		"slotLabel":      effect.SlotLabel,
	})
	return err
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
	if err := json.Unmarshal([]byte(botSession.Payload), &payload); err != nil || strings.TrimSpace(payload.BookingID) == "" {
		_ = s.runtime.services.BotSessions.ClearSession(ctx, domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}, workspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID)
		return nil
	}

	actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: workspaceID}
	if _, err := s.runtime.services.Bookings.ConfirmBooking(ctx, actor, workspaceID, payload.BookingID, 0, firstNonEmptyString(payload.ConversationID, conversation.ID)); err != nil {
		if strings.Contains(err.Error(), "slot unavailable") {
			return s.enqueueTelegramCalendarReply(ctx, account, inbound.chatID, workspaceID, conversation.ID, "Слот уже недоступен. Откройте календарь и выберите другое время.", inbound.messageID, inbound.callbackID, inbound.callbackData)
		}
		return err
	}
	if err := s.runtime.services.BotSessions.ClearSession(ctx, actor, workspaceID, string(BotSessionScopeClient), string(BotSessionActorCustomer), customer.ID); err != nil {
		return err
	}
	s.notifyOperatorsAboutConversation(ctx, conversation, customer)
	return nil
}

func (s *Server) applyBotEngineClientBookingCancelled(ctx context.Context, inbound telegramClientInbound, effect botEngineEffect) error {
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
		ExpiresAt:        time.Now().UTC().Add(botRuntimeClientRouteTTL),
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
	return s.runtime.services.Inbox.ReceiveInboundMessageForWorkspace(ctx, workspaceID, inboundUsecaseInput(
		ChannelTelegram,
		account.ID,
		inbound.chatID,
		inbound.externalMessageID,
		text,
		inbound.timestamp,
		inbound.profile,
	))
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

func (s *Server) enqueueBotEngineReply(ctx context.Context, account ChannelAccount, chatID string, state *botEngineClientSession, effect botEngineEffect, inboundMessageID int64, callbackID, callbackData string) error {
	payload := TelegramOutboundPayload{
		ChatID:  chatID,
		Text:    effect.Text,
		Buttons: s.buildBotEngineReplyButtons(account, chatID, state, effect.Buttons),
	}
	kind := OutboundKindTelegramSendText
	if len(payload.Buttons) > 0 {
		kind = OutboundKindTelegramSendInline
	}
	if account.ChannelKind == ChannelKindTelegramOperator && s != nil && s.runtime != nil && s.runtime.redis != nil {
		replyKey := telegramOperatorReplyKey(account.ID, chatID, inboundMessageID, callbackData, payload)
		if replyKey != "" {
			freshReply, err := s.runtime.redis.SetNX(ctx, replyKey, "1", telegramOperatorReplyCooldown).Result()
			if err != nil {
				return err
			}
			if !freshReply {
				log.Printf("telegram operator reply skipped duplicate account_id=%s chat_id=%s buttons=%d text=%q", account.ID, chatID, len(payload.Buttons), telegramLogValue(payload.Text))
				return nil
			}
		}
	}
	return s.enqueueTelegramOutbound(ctx, account, kind, "", "", payload, inboundMessageID, callbackID, callbackData)
}

func (s *Server) buildBotEngineReplyButtons(account ChannelAccount, chatID string, state *botEngineClientSession, buttons []botEngineButton) []TelegramInlineButton {
	workspaceID := strings.TrimSpace(account.WorkspaceID)
	if state != nil && state.Route.Kind == "ready" && strings.TrimSpace(state.Route.WorkspaceID) != "" {
		workspaceID = strings.TrimSpace(state.Route.WorkspaceID)
	}
	result := make([]TelegramInlineButton, 0, len(buttons))
	for _, button := range buttons {
		text := strings.TrimSpace(button.Text)
		action := strings.TrimSpace(button.Action)
		if text == "" || action == "" {
			continue
		}
		if action == "client:open_calendar" {
			if workspaceID == "" {
				continue
			}
			link, err := s.clientCalendarURL(account, chatID, workspaceID)
			if err != nil {
				log.Printf("telegram calendar url build failed account_id=%s workspace_id=%s chat_id=%s error=%v", account.ID, workspaceID, chatID, err)
				continue
			}
			result = append(result, TelegramInlineButton{Text: text, URL: link})
			continue
		}
		result = append(result, TelegramInlineButton{Text: text, CallbackData: action})
	}
	return result
}

func (s *Server) enqueueTelegramCalendarReply(ctx context.Context, account ChannelAccount, chatID, workspaceID, conversationID, text string, inboundMessageID int64, callbackID, callbackData string) error {
	payload := TelegramOutboundPayload{
		ChatID: chatID,
		Text:   text,
	}
	if strings.TrimSpace(workspaceID) != "" {
		link, err := s.clientCalendarURL(account, chatID, workspaceID)
		if err != nil {
			log.Printf("telegram calendar reply url build failed account_id=%s workspace_id=%s chat_id=%s error=%v", account.ID, workspaceID, chatID, err)
		} else {
			payload.Buttons = []TelegramInlineButton{{Text: "Открыть календарь", URL: link}}
		}
	}
	kind := OutboundKindTelegramSendText
	if len(payload.Buttons) > 0 {
		kind = OutboundKindTelegramSendInline
	}
	return s.enqueueTelegramOutbound(ctx, account, kind, conversationID, "", payload, inboundMessageID, callbackID, callbackData)
}

func (s *Server) buildBotEngineOperatorContext(ctx context.Context, binding OperatorBotBinding, linkBindings []botEngineOperatorLinkBinding, scope botEngineOperatorContextScope) (botEngineOperatorContext, *botEngineOperatorWorkspace, error) {
	contextState := botEngineOperatorContext{
		LinkBindings: append([]botEngineOperatorLinkBinding(nil), linkBindings...),
		Workspaces:   []botEngineOperatorWorkspace{},
	}
	type workspaceRequest struct {
		userID string
		chatID string
	}
	workspaceRequests := map[string]workspaceRequest{}
	if strings.TrimSpace(binding.WorkspaceID) != "" {
		workspaceRequests[binding.WorkspaceID] = workspaceRequest{
			userID: binding.UserID,
			chatID: binding.TelegramChatID,
		}
	}
	for _, linkBinding := range linkBindings {
		if strings.TrimSpace(linkBinding.WorkspaceID) == "" {
			continue
		}
		if _, exists := workspaceRequests[linkBinding.WorkspaceID]; !exists {
			workspaceRequests[linkBinding.WorkspaceID] = workspaceRequest{
				userID: linkBinding.UserID,
				chatID: linkBinding.ChatID,
			}
		}
	}

	workspaceIDs := make([]string, 0, len(workspaceRequests))
	for workspaceID := range workspaceRequests {
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	sort.Strings(workspaceIDs)

	var currentWorkspace *botEngineOperatorWorkspace
	for _, workspaceID := range workspaceIDs {
		req := workspaceRequests[workspaceID]
		workspaceState, err := s.buildBotEngineOperatorWorkspace(ctx, workspaceID, req.userID, req.chatID, scope)
		if err != nil {
			return botEngineOperatorContext{}, nil, err
		}
		contextState.Workspaces = append(contextState.Workspaces, *workspaceState)
		if workspaceID == binding.WorkspaceID {
			workspaceCopy := *workspaceState
			currentWorkspace = &workspaceCopy
		}
	}
	return contextState, currentWorkspace, nil
}

func (s *Server) buildBotEngineOperatorWorkspace(ctx context.Context, workspaceID, userID, chatID string, scope botEngineOperatorContextScope) (*botEngineOperatorWorkspace, error) {
	workspace, err := s.runtime.repository.WorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	workspaceState := &botEngineOperatorWorkspace{
		ID:            workspace.ID,
		Name:          workspace.Name,
		Dashboard:     botEngineOperatorDashboard{NextSlot: "нет"},
		Conversations: []botEngineOperatorConversation{},
		WeekSlots:     []botEngineOperatorWeekSlot{},
		Reminders:     []botEngineOperatorReminder{},
		Settings: botEngineOperatorSettings{
			TelegramChatLabel: chatID,
			FAQQuestions:      []string{},
		},
	}

	now := time.Now().UTC()
	var availableSlots []DailySlot
	if scope.Dashboard || scope.Conversations {
		availableSlots, err = s.runtime.repository.AvailableDaySlots(ctx, workspaceID, now, now.AddDate(0, 0, 14))
		if err != nil {
			return nil, err
		}
	}
	if scope.Dashboard {
		dashboard, err := s.runtime.repository.Dashboard(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		bookings, err := s.runtime.repository.Bookings(ctx, workspaceID, "all")
		if err != nil {
			return nil, err
		}
		workspaceLoc := time.UTC
		if workspace.Timezone != "" {
			if loc, loadErr := time.LoadLocation(workspace.Timezone); loadErr == nil {
				workspaceLoc = loc
			}
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
			if nextSlot == "нет" && booking.StartsAt.After(now) {
				nextSlot = booking.StartsAt.In(workspaceLoc).Format("Mon 02.01 15:04")
			}
		}
		if nextSlot == "нет" && len(availableSlots) > 0 {
			nextSlot = availableSlots[0].StartsAt.In(workspaceLoc).Format("Mon 02.01 15:04")
		}
		workspaceState.Dashboard = botEngineOperatorDashboard{
			TodayBookings: dashboard.TodayBookings,
			NewMessages:   dashboard.NewMessages,
			Revenue:       revenue,
			FreeSlots:     len(availableSlots),
			NextSlot:      nextSlot,
		}
	}
	if scope.Conversations {
		conversations, err := s.runtime.repository.Conversations(ctx, workspaceID)
		if err != nil {
			return nil, err
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
			customer, customerErr := s.runtime.repository.Customer(ctx, workspaceID, conversation.CustomerID)
			if customerErr == nil {
				item.CustomerName = firstNonEmptyString(customer.Name, item.CustomerName)
				item.CustomerPhone = customer.Phone
			}
			operatorConversations = append(operatorConversations, item)
		}
		workspaceState.Conversations = operatorConversations
	}
	if scope.WeekSlots {
		weekSlots, err := s.runtime.repository.WeekSlots(ctx, workspaceID, now)
		if err != nil {
			return nil, err
		}
		workspaceState.WeekSlots = make([]botEngineOperatorWeekSlot, 0, len(weekSlots))
		for _, day := range weekSlots {
			workspaceState.WeekSlots = append(workspaceState.WeekSlots, botEngineOperatorWeekSlot{
				Label:     day.Label,
				SlotCount: len(day.Slots),
			})
		}
	}
	if scope.Reminders {
		reminderBookings, err := s.runtime.repository.UpcomingReminderBookings(ctx, workspaceID, now, operatorReminderHorizon, operatorReminderListLimit)
		if err != nil {
			return nil, err
		}
		workspaceState.Reminders = make([]botEngineOperatorReminder, 0, len(reminderBookings))
		for _, booking := range reminderBookings {
			workspaceState.Reminders = append(workspaceState.Reminders, botEngineOperatorReminder{
				BookingID:     booking.ID,
				CustomerName:  booking.CustomerName,
				StartsAtLabel: formatBookingReminderTime(booking.StartsAt, workspace.Timezone),
				Enabled:       booking.ClientReminderEnabled,
				Sent:          booking.ClientReminderSentAt != nil,
			})
		}
	}
	if scope.Settings || scope.FAQ || scope.Dashboard {
		operatorSettings, err := s.runtime.repository.OperatorBotSettings(ctx, workspaceID, userID, s.cfg.OperatorBotUsername, s.cfg.PublicBaseURL)
		if err != nil {
			return nil, err
		}
		botConfig, faqItems, err := s.runtime.repository.BotConfig(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
		workspaceState.Settings.AutoReply = botConfig.AutoReply
		workspaceState.Settings.HandoffEnabled = botConfig.HandoffEnabled
		workspaceState.Settings.WebhookURL = operatorSettings.OperatorWebhookURL
		if operatorSettings.Binding != nil && strings.TrimSpace(operatorSettings.Binding.TelegramChatID) != "" {
			workspaceState.Settings.TelegramChatLabel = operatorSettings.Binding.TelegramChatID
		}
		if scope.FAQ {
			for _, item := range faqItems {
				if strings.TrimSpace(item.Question) != "" {
					workspaceState.Settings.FAQQuestions = append(workspaceState.Settings.FAQQuestions, item.Question)
				}
			}
		}
	}
	return workspaceState, nil
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

func (s *Server) applyBotEngineOperatorTransition(ctx context.Context, account ChannelAccount, snapshot botRuntimeOperatorSnap, transition botEngineOperatorTransition) error {
	binding := operatorBindingFromState(transition.State.Binding)
	for _, effect := range transition.Effects {
		if err := s.applyBotEngineOperatorEffect(ctx, account, snapshot, &binding, effect); err != nil {
			return err
		}
	}
	return s.persistBotEngineOperatorSession(ctx, binding, transition.State)
}

func (s *Server) applyBotEngineOperatorEffect(ctx context.Context, account ChannelAccount, snapshot botRuntimeOperatorSnap, binding *OperatorBotBinding, effect botEngineEffect) error {
	switch effect.Type {
	case "reply":
		return s.enqueueBotEngineReply(ctx, account, snapshot.ChatID, nil, effect, snapshot.MessageID, snapshot.CallbackID, snapshot.CallbackData)
	case "configuration.error":
		log.Printf("telegram bot runtime configuration error bot=%s workspace_id=%s message=%q", effect.Bot, effect.WorkspaceID, effect.Message)
		return nil
	case "operator.bound":
		code := strings.TrimSpace(effect.Code)
		if code == "" {
			return errors.New("operator bind effect is missing link code")
		}
		result, err := s.runtime.services.OperatorLink.LinkTelegram(ctx, code, snapshot.TelegramUserID, firstNonEmptyString(effect.ChatID, snapshot.ChatID))
		if err != nil {
			return err
		}
		*binding = OperatorBotBinding{
			UserID:         result.UserID,
			WorkspaceID:    result.WorkspaceID,
			TelegramUserID: result.TelegramUserID,
			TelegramChatID: result.TelegramChatID,
			LinkedAt:       time.Now().UTC(),
			IsActive:       true,
		}
		return nil
	case "crm":
		return s.applyBotEngineOperatorCRMEffect(ctx, *binding, effect)
	case "settings.auto_reply_changed":
		if strings.TrimSpace(binding.WorkspaceID) == "" {
			return nil
		}
		actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: binding.WorkspaceID, UserID: binding.UserID}
		return s.runtime.services.BotSettings.ToggleAutoReply(ctx, actor, binding.WorkspaceID, effect.Enabled)
	case "booking.client_reminder_changed":
		if strings.TrimSpace(binding.WorkspaceID) == "" || strings.TrimSpace(effect.BookingID) == "" {
			return nil
		}
		_, err := s.runtime.repository.SetBookingClientReminderEnabled(ctx, binding.WorkspaceID, effect.BookingID, effect.Enabled)
		return err
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
			ExpiresAt:   time.Now().UTC().Add(botRuntimeOperatorSessionTTL),
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
			ExpiresAt:   time.Now().UTC().Add(botRuntimeOperatorSessionTTL),
		}, payload)
		return err
	default:
		return s.runtime.services.BotSessions.ClearSession(ctx, actor, binding.WorkspaceID, string(BotSessionScopeOperator), string(BotSessionActorUser), binding.UserID)
	}
}

func (s *Server) syncTelegramWebhook(ctx context.Context, account ChannelAccount) error {
	if account.Provider != ChannelTelegram {
		return nil
	}
	token, err := decryptString(s.cfg.EncryptionSecret, account.EncryptedToken)
	if err != nil {
		return fmt.Errorf("failed to decrypt telegram bot token")
	}
	if strings.TrimSpace(token) == "" {
		if account.Connected || account.IsEnabled {
			return errors.New("telegram bot token is not configured")
		}
		return nil
	}
	if !account.Connected || !account.IsEnabled {
		if err := s.runtime.telegram.DeleteWebhook(ctx, token, false); err != nil {
			return fmt.Errorf("failed to delete telegram webhook: %w", err)
		}
		return nil
	}
	webhookURL, err := s.telegramWebhookURL(account)
	if err != nil {
		return err
	}
	request := tgapi.SetWebhookRequest{
		URL:            webhookURL,
		AllowedUpdates: []string{"message", "callback_query"},
	}
	if account.ChannelKind == ChannelKindTelegramOperator {
		request.SecretToken = strings.TrimSpace(account.WebhookSecret)
		if request.SecretToken == "" {
			return errors.New("telegram webhook secret is not configured")
		}
	}
	if err := s.runtime.telegram.SetWebhook(ctx, token, request); err != nil {
		return fmt.Errorf("failed to register telegram webhook: %w", err)
	}
	return nil
}

func (s *Server) telegramWebhookURL(account ChannelAccount) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.PublicBaseURL), "/")
	if baseURL == "" {
		return "", errors.New("PUBLIC_BASE_URL must be configured")
	}
	switch account.ChannelKind {
	case ChannelKindTelegramClient:
		if strings.TrimSpace(account.WebhookSecret) == "" {
			return "", errors.New("telegram client webhook secret is not configured")
		}
		return baseURL + fmt.Sprintf("/webhooks/telegram/client/%s/%s", account.ID, account.WebhookSecret), nil
	case ChannelKindTelegramOperator:
		return baseURL + "/webhooks/telegram/operator", nil
	default:
		return "", fmt.Errorf("telegram webhook is not supported for channel kind %s", account.ChannelKind)
	}
}

func (s *Server) enqueueTelegramOutbound(ctx context.Context, account ChannelAccount, kind OutboundKind, conversationID, messageID string, payload TelegramOutboundPayload, inboundMessageID int64, callbackID, callbackData string) error {
	if account.ID == "" {
		return errors.New("telegram channel account is empty")
	}
	dedupKey := telegramOutboundDedupKey(account, payload.ChatID, inboundMessageID, callbackID, callbackData, kind, conversationID, messageID, payload)
	_, inserted, err := s.runtime.repository.EnqueueOutboundMessage(ctx, OutboundMessage{
		WorkspaceID:      account.WorkspaceID,
		Channel:          account.Provider,
		ChannelKind:      account.ChannelKind,
		ChannelAccountID: account.ID,
		ConversationID:   conversationID,
		MessageID:        messageID,
		DedupKey:         dedupKey,
		Kind:             kind,
	}, payload)
	if err != nil {
		return err
	}
	if !inserted {
		log.Printf(
			"telegram outbound skipped duplicate bot=%s kind=%s account_id=%s conversation_id=%s message_id=%s chat_id=%s buttons=%d text=%q",
			account.ChannelKind,
			kind,
			account.ID,
			conversationID,
			messageID,
			payload.ChatID,
			len(payload.Buttons),
			telegramLogValue(payload.Text),
		)
		return nil
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
	select {
	case s.outboundWake <- struct{}{}:
	default:
	}
	return nil
}

func (s *Server) enqueueTelegramMasterPhonePrompt(ctx context.Context, account ChannelAccount, chatID string, welcome bool, inboundMessageID int64, callbackID, callbackData string) error {
	if s.runtime == nil || s.runtime.redis == nil {
		return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
			ChatID: chatID,
			Text:   telegramClientPromptText(welcome),
			Buttons: []TelegramInlineButton{
				{Text: "Ввести номер мастера", CallbackData: "client:enter_master_phone"},
			},
		}, inboundMessageID, callbackID, callbackData)
	}
	text := telegramClientPromptText(welcome)
	freshPrompt, err := s.runtime.redis.SetNX(ctx, telegramClientPromptKey(account.ID, chatID, text), "1", telegramPromptCooldown).Result()
	if err != nil {
		return err
	}
	if !freshPrompt {
		return nil
	}
	return s.enqueueTelegramOutbound(ctx, account, OutboundKindTelegramSendInline, "", "", TelegramOutboundPayload{
		ChatID: chatID,
		Text:   text,
		Buttons: []TelegramInlineButton{
			{Text: "Ввести номер мастера", CallbackData: "client:enter_master_phone"},
		},
	}, inboundMessageID, callbackID, callbackData)
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
		}, 0, "", "")
	}
}

func (s *Server) claimTelegramCallbackAction(ctx context.Context, accountID string, botKind ChannelKind, chatID string, messageID int64, data string) (bool, error) {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(chatID) == "" || messageID == 0 || strings.TrimSpace(data) == "" {
		return true, nil
	}
	if s == nil || s.runtime == nil || s.runtime.redis == nil {
		return true, nil
	}
	return s.runtime.redis.SetNX(ctx, telegramCallbackActionKey(accountID, botKind, chatID, messageID, data), "1", telegramCallbackActionCooldown).Result()
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
	return s.runtime.redis.SetNX(ctx, telegramInboundDeliveryKey(accountID, botKind, chatID, messageID, callbackID), "1", telegramInboundDeliveryCooldown).Result()
}

func (s *Server) claimTelegramOperatorCommand(ctx context.Context, accountID, chatID, command string) (bool, error) {
	if !operatorCommandNeedsThrottle(command) {
		return true, nil
	}
	if s == nil || s.runtime == nil || s.runtime.redis == nil {
		return true, nil
	}
	return s.runtime.redis.SetNX(ctx, telegramOperatorCommandKey(accountID, chatID, command), "1", telegramOperatorCommandCooldown).Result()
}

func (s *Server) releaseTelegramDedupKey(ctx context.Context, key string) {
	if strings.TrimSpace(key) == "" || s == nil || s.runtime == nil || s.runtime.redis == nil {
		return
	}
	_ = s.runtime.redis.Del(ctx, key).Err()
}

func telegramCallbackActionKey(accountID string, botKind ChannelKind, chatID string, messageID int64, data string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(data)))
	return fmt.Sprintf("tg:cbq:%s:%s:%s:%d:%s", strings.TrimSpace(accountID), strings.TrimSpace(string(botKind)), strings.TrimSpace(chatID), messageID, hex.EncodeToString(hash[:8]))
}

func telegramInboundDeliveryKey(accountID string, botKind ChannelKind, chatID string, messageID int64, callbackID string) string {
	if trimmed := strings.TrimSpace(callbackID); trimmed != "" {
		hash := sha256.Sum256([]byte(trimmed))
		return fmt.Sprintf("tg:upd:%s:%s:%s:cbq:%s", strings.TrimSpace(accountID), strings.TrimSpace(string(botKind)), strings.TrimSpace(chatID), hex.EncodeToString(hash[:8]))
	}
	return fmt.Sprintf("tg:upd:%s:%s:%s:msg:%d", strings.TrimSpace(accountID), strings.TrimSpace(string(botKind)), strings.TrimSpace(chatID), messageID)
}

func telegramOutboundDedupKey(account ChannelAccount, chatID string, inboundMessageID int64, callbackID, callbackData string, kind OutboundKind, conversationID, messageID string, payload TelegramOutboundPayload) string {
	if strings.TrimSpace(account.ID) == "" || strings.TrimSpace(chatID) == "" {
		return ""
	}
	identity := telegramOutboundDedupIdentity(inboundMessageID, callbackID)
	if identity == "" {
		return ""
	}
	signaturePayload, err := json.Marshal(struct {
		Kind           OutboundKind           `json:"kind"`
		ConversationID string                 `json:"conversationId,omitempty"`
		MessageID      string                 `json:"messageId,omitempty"`
		Text           string                 `json:"text,omitempty"`
		TargetMessage  int64                  `json:"targetMessage,omitempty"`
		Buttons        []TelegramInlineButton `json:"buttons,omitempty"`
		ParseMode      string                 `json:"parseMode,omitempty"`
		CallbackText   string                 `json:"callbackText,omitempty"`
		ShowAlert      bool                   `json:"showAlert,omitempty"`
	}{
		Kind:           kind,
		ConversationID: strings.TrimSpace(conversationID),
		MessageID:      strings.TrimSpace(messageID),
		Text:           strings.TrimSpace(payload.Text),
		TargetMessage:  payload.MessageID,
		Buttons:        payload.Buttons,
		ParseMode:      strings.TrimSpace(payload.ParseMode),
		CallbackText:   strings.TrimSpace(payload.CallbackText),
		ShowAlert:      payload.ShowAlert,
	})
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(signaturePayload)
	return fmt.Sprintf(
		"tg:out:%s:%s:%s:%s:%s",
		strings.TrimSpace(account.ID),
		strings.TrimSpace(string(account.ChannelKind)),
		strings.TrimSpace(chatID),
		identity,
		hex.EncodeToString(hash[:8]),
	)
}

func telegramOutboundDedupIdentity(messageID int64, callbackID string) string {
	if trimmed := strings.TrimSpace(callbackID); trimmed != "" {
		hash := sha256.Sum256([]byte(trimmed))
		return "cbq:" + hex.EncodeToString(hash[:8])
	}
	if messageID == 0 {
		return ""
	}
	return "msg:" + strconv.FormatInt(messageID, 10)
}

func telegramClientPromptKey(accountID, chatID, text string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return fmt.Sprintf("tg:prompt:%s:%s:%s", strings.TrimSpace(accountID), strings.TrimSpace(chatID), hex.EncodeToString(hash[:8]))
}

func telegramOperatorCommandKey(accountID, chatID, command string) string {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(chatID) == "" || strings.TrimSpace(command) == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(strings.TrimSpace(command)))
	return fmt.Sprintf("tg:opcmd:%s:%s:%s", strings.TrimSpace(accountID), strings.TrimSpace(chatID), hex.EncodeToString(hash[:8]))
}

func telegramOperatorReplyKey(accountID, chatID string, inboundMessageID int64, callbackData string, payload TelegramOutboundPayload) string {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(chatID) == "" {
		return ""
	}
	identity := ""
	if inboundMessageID != 0 {
		if trimmed := strings.TrimSpace(callbackData); trimmed != "" {
			hash := sha256.Sum256([]byte(trimmed))
			identity = "cbqmsg:" + strconv.FormatInt(inboundMessageID, 10) + ":" + hex.EncodeToString(hash[:8])
		} else {
			identity = "msg:" + strconv.FormatInt(inboundMessageID, 10)
		}
	}
	if identity == "" {
		return ""
	}
	signaturePayload, err := json.Marshal(struct {
		Text      string                 `json:"text,omitempty"`
		Buttons   []TelegramInlineButton `json:"buttons,omitempty"`
		ParseMode string                 `json:"parseMode,omitempty"`
	}{
		Text:      strings.TrimSpace(payload.Text),
		Buttons:   payload.Buttons,
		ParseMode: strings.TrimSpace(payload.ParseMode),
	})
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(signaturePayload)
	return fmt.Sprintf("tg:opreply:%s:%s:%s:%s", strings.TrimSpace(accountID), strings.TrimSpace(chatID), identity, hex.EncodeToString(hash[:8]))
}

func operatorCommandNeedsThrottle(command string) bool {
	command = strings.TrimSpace(strings.ToLower(command))
	if command == "" {
		return false
	}
	return command == "отмена" || strings.HasPrefix(command, "/")
}

func operatorContextScopeForEvent(event botEngineOperatorEvent) botEngineOperatorContextScope {
	command := ""
	switch event.Type {
	case "callback":
		command = strings.TrimSpace(event.Data)
	case "message":
		command = strings.TrimSpace(event.Text)
	default:
		return botEngineOperatorContextScope{}
	}
	command = normalizeOperatorCommand(command)
	scope := botEngineOperatorContextScope{}
	switch {
	case command == "/dashboard":
		scope.Dashboard = true
		scope.Settings = true
	case command == "/dialogs" ||
		strings.HasPrefix(command, "/dialog ") ||
		strings.HasPrefix(command, "dialog:") ||
		strings.HasPrefix(command, "reply:") ||
		strings.HasPrefix(command, "slots:") ||
		strings.HasPrefix(command, "pickslot:") ||
		strings.HasPrefix(command, "take:") ||
		strings.HasPrefix(command, "auto:") ||
		strings.HasPrefix(command, "close:"):
		scope.Conversations = true
	case command == "/slots":
		scope.WeekSlots = true
	case command == "/reminders" || strings.HasPrefix(command, "reminder:toggle:"):
		scope.Reminders = true
	case command == "/settings" || command == "/auto_on" || command == "/auto_off":
		scope.Settings = true
	case command == "/faq":
		scope.FAQ = true
	}
	return scope
}

func telegramClientPromptText(welcome bool) string {
	if welcome {
		return "Здравствуйте!\nЯ помогу связаться с нужным мастером и передам ваши сообщения в CRM.\n\nВведите номер мастера, к которому хотите записаться."
	}
	return "Введите номер мастера, к которому хотите записаться."
}

func telegramStartPayload(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	command, rest, _ := strings.Cut(text, " ")
	command = strings.SplitN(strings.TrimSpace(command), "@", 2)[0]
	if !strings.EqualFold(command, "/start") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func telegramMasterPhoneFromStartPayload(payload string) string {
	payload = strings.TrimSpace(payload)
	switch {
	case strings.HasPrefix(strings.ToLower(payload), "master_"):
		return strings.TrimSpace(payload[len("master_"):])
	case strings.HasPrefix(strings.ToLower(payload), "phone_"):
		return strings.TrimSpace(payload[len("phone_"):])
	default:
		return payload
	}
}

func operatorLinkCodeFromInput(rawText string, allowPlainCode bool) string {
	if payload, isStart := telegramStartPayload(rawText); isStart {
		return strings.TrimSpace(payload)
	}
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" || !allowPlainCode || strings.HasPrefix(trimmed, "/") {
		return ""
	}
	return trimmed
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
	case strings.HasPrefix(text, "rmd:toggle:"):
		parts := strings.Split(text, ":")
		if len(parts) == 4 {
			return "reminder:toggle:" + parts[2] + ":" + parts[3]
		}
	}
	return text
}

func telegramOperatorCommandForUpdate(update tgapi.Update, text string) string {
	if update.CallbackQuery != nil {
		command := normalizeOperatorCommand(callbackDataFromUpdate(update))
		if operatorCommandNeedsThrottle(command) {
			return command
		}
		return ""
	}
	return normalizeOperatorCommand(text)
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

func callbackDataFromUpdate(update tgapi.Update) string {
	if update.CallbackQuery != nil {
		return strings.TrimSpace(update.CallbackQuery.Data)
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

func botRuntimeEventIDFromUpdate(update tgapi.Update) string {
	if update.UpdateID != 0 {
		return fmt.Sprintf("telegram:update:%d", update.UpdateID)
	}
	if callbackID := callbackIDFromUpdate(update); callbackID != "" {
		return "telegram:callback:" + callbackID
	}
	if messageID := messageIDFromUpdate(update); messageID != 0 {
		return fmt.Sprintf("telegram:message:%d", messageID)
	}
	return ""
}

func operatorBindingFromState(state botEngineOperatorBindingState) OperatorBotBinding {
	if state.Kind != "bound" {
		return OperatorBotBinding{}
	}
	return OperatorBotBinding{
		WorkspaceID:    state.WorkspaceID,
		UserID:         state.UserID,
		TelegramChatID: state.ChatID,
		IsActive:       true,
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
	return startsAt.UTC().Format("02.01 15:04")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func telegramLogValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}
