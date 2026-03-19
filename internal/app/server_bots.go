package app

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
	"github.com/vital/rendycrm-app/internal/usecase"
)

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
		settings, err := s.runtime.repository.OperatorBotSettings(r.Context(), auth.Workspace.ID, auth.User.ID, s.cfg.OperatorBotUsername, s.cfg.PublicBaseURL)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "operator bot settings query failed")
			return
		}
		code, err := s.runtime.services.OperatorLink.CreateLinkCode(r.Context(), actor, auth.Workspace.ID, auth.User.ID, settings.BotUsername)
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
		if err := s.syncTelegramWebhook(r.Context(), account); err != nil {
			s.writeError(w, http.StatusBadGateway, err.Error())
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
	if !result.Stored {
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	_ = s.publishEvent(r.Context(), SSEEvent{Type: EventMessageNew, Data: Message{ID: result.MessageID, ConversationID: result.ConversationID}})
	_ = s.publishDashboard(r.Context(), result.WorkspaceID)
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
