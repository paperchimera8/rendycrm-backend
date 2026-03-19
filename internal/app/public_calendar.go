package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
	"github.com/vital/rendycrm-app/internal/usecase"
)

const publicCalendarTokenTTL = 30 * 24 * time.Hour
const publicCalendarMaxRangeDays = 31

type publicCalendarAccessToken struct {
	WorkspaceID      string    `json:"workspaceId"`
	ChannelAccountID string    `json:"channelAccountId"`
	ExternalChatID   string    `json:"externalChatId"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

func (s *Server) encodePublicCalendarAccessToken(payload publicCalendarAccessToken) (string, error) {
	if strings.TrimSpace(payload.WorkspaceID) == "" {
		return "", errors.New("workspace id is required")
	}
	if strings.TrimSpace(payload.ChannelAccountID) == "" {
		return "", errors.New("channel account id is required")
	}
	if strings.TrimSpace(payload.ExternalChatID) == "" {
		return "", errors.New("external chat id is required")
	}
	if payload.ExpiresAt.IsZero() {
		payload.ExpiresAt = time.Now().UTC().Add(publicCalendarTokenTTL)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return encryptString(s.cfg.EncryptionSecret, string(raw))
}

func (s *Server) decodePublicCalendarAccessToken(raw string) (publicCalendarAccessToken, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return publicCalendarAccessToken{}, errors.New("missing token")
	}
	plaintext, err := decryptString(s.cfg.EncryptionSecret, raw)
	if err != nil {
		return publicCalendarAccessToken{}, errors.New("invalid token")
	}
	var payload publicCalendarAccessToken
	if err := json.Unmarshal([]byte(plaintext), &payload); err != nil {
		return publicCalendarAccessToken{}, errors.New("invalid token")
	}
	if strings.TrimSpace(payload.WorkspaceID) == "" || strings.TrimSpace(payload.ChannelAccountID) == "" || strings.TrimSpace(payload.ExternalChatID) == "" {
		return publicCalendarAccessToken{}, errors.New("invalid token")
	}
	if !payload.ExpiresAt.IsZero() && payload.ExpiresAt.Before(time.Now().UTC()) {
		return publicCalendarAccessToken{}, errors.New("token expired")
	}
	return payload, nil
}

func publicCalendarURL(baseURL, appBasePath, token string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", errors.New("public base url is empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	currentPath := strings.TrimRight(parsed.Path, "/")
	appRoot := currentPath
	normalizedBase := strings.TrimRight(strings.TrimSpace(appBasePath), "/")
	if normalizedBase != "" {
		if appRoot == "" || appRoot == "/" {
			appRoot = normalizedBase
		} else if !strings.HasSuffix(appRoot, normalizedBase) {
			appRoot = strings.TrimRight(appRoot, "/") + normalizedBase
		}
	}
	parsed.Path = strings.TrimRight(appRoot, "/") + "/calendar"
	values := parsed.Query()
	values.Set("token", token)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func (s *Server) clientCalendarURL(account ChannelAccount, chatID, workspaceID string) (string, error) {
	token, err := s.encodePublicCalendarAccessToken(publicCalendarAccessToken{
		WorkspaceID:      workspaceID,
		ChannelAccountID: account.ID,
		ExternalChatID:   chatID,
		ExpiresAt:        time.Now().UTC().Add(publicCalendarTokenTTL),
	})
	if err != nil {
		return "", err
	}
	return publicCalendarURL(s.cfg.PublicBaseURL, s.cfg.AppBasePath, token)
}

func (s *Server) handlePublicCalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	access, err := s.decodePublicCalendarAccessToken(r.URL.Query().Get("token"))
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	dateFrom := time.Now().UTC()
	dateTo := dateFrom.AddDate(0, 0, 14)
	if raw := strings.TrimSpace(r.URL.Query().Get("date_from")); raw != "" {
		parsed, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			s.writeError(w, http.StatusBadRequest, "invalid date_from")
			return
		}
		dateFrom = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("date_to")); raw != "" {
		parsed, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			s.writeError(w, http.StatusBadRequest, "invalid date_to")
			return
		}
		dateTo = parsed
	}
	if dateTo.Before(dateFrom) {
		s.writeError(w, http.StatusBadRequest, "date_to must not be before date_from")
		return
	}
	if dateTo.Sub(dateFrom) > publicCalendarMaxRangeDays*24*time.Hour {
		s.writeError(w, http.StatusBadRequest, "date range is too large")
		return
	}

	workspace, err := s.runtime.repository.Workspace(r.Context(), access.WorkspaceID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	settings, err := s.runtime.repository.SlotSettings(r.Context(), access.WorkspaceID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "slot settings query failed")
		return
	}
	items, err := s.runtime.repository.AvailableDaySlots(r.Context(), access.WorkspaceID, dateFrom, dateTo)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "available slots query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"workspace": map[string]any{
			"id":       workspace.ID,
			"name":     workspace.Name,
			"timezone": settings.Timezone,
		},
		"items": items,
	})
}

func (s *Server) handlePublicCalendarBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Token       string `json:"token"`
		DailySlotID string `json:"dailySlotId"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	access, err := s.decodePublicCalendarAccessToken(payload.Token)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if strings.TrimSpace(payload.DailySlotID) == "" {
		s.writeError(w, http.StatusBadRequest, "dailySlotId is required")
		return
	}

	account, err := s.runtime.repository.ChannelAccountByID(r.Context(), access.ChannelAccountID)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "channel account not found")
		return
	}
	if account.Provider != ChannelTelegram || account.ChannelKind != ChannelKindTelegramClient {
		s.writeError(w, http.StatusUnauthorized, "channel account is invalid")
		return
	}

	customer, err := s.runtime.repository.EnsureCustomerIdentity(r.Context(), access.WorkspaceID, ChannelTelegram, access.ExternalChatID, InboundProfile{})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to resolve customer")
		return
	}

	actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: access.WorkspaceID}
	created, err := s.runtime.services.Bookings.CreateBooking(r.Context(), actor, usecase.CreateBookingInput{
		WorkspaceID: access.WorkspaceID,
		CustomerID:  customer.ID,
		DailySlotID: payload.DailySlotID,
		Status:      "pending",
		Notes:       "Создано через public calendar",
	})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "slot unavailable") {
			status = http.StatusConflict
		}
		s.writeError(w, status, err.Error())
		return
	}

	confirmed, err := s.runtime.services.Bookings.ConfirmBooking(r.Context(), actor, access.WorkspaceID, created.ID, 0, "")
	if err != nil {
		_, _ = s.runtime.services.Bookings.CancelBooking(r.Context(), actor, access.WorkspaceID, created.ID, "")
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "slot unavailable") {
			status = http.StatusConflict
		}
		s.writeError(w, status, err.Error())
		return
	}

	booking, err := s.runtime.repository.Booking(r.Context(), access.WorkspaceID, confirmed.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "booking query failed")
		return
	}

	_ = s.publishEvent(r.Context(), SSEEvent{Type: EventBookingUpdated, Data: booking})
	_ = s.publishDashboard(r.Context(), access.WorkspaceID)
	_ = s.runtime.jobs.Enqueue(r.Context(), "analytics.refresh", map[string]string{"workspaceId": access.WorkspaceID})

	if messageText, msgErr := s.publicCalendarConfirmationText(r.Context(), access.WorkspaceID, booking); msgErr == nil {
		if outboundErr := s.enqueueTelegramOutbound(r.Context(), account, OutboundKindTelegramSendText, "", "", TelegramOutboundPayload{
			ChatID: access.ExternalChatID,
			Text:   messageText,
		}); outboundErr != nil {
			log.Printf("public calendar confirmation enqueue failed workspace=%s slot=%s chat_id=%s error=%v", access.WorkspaceID, payload.DailySlotID, access.ExternalChatID, outboundErr)
		}
	}

	s.writeJSON(w, http.StatusCreated, map[string]any{"booking": booking})
}

func (s *Server) publicCalendarConfirmationText(ctx context.Context, workspaceID string, booking Booking) (string, error) {
	settings, err := s.runtime.repository.SlotSettings(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	location := time.UTC
	if tz := strings.TrimSpace(settings.Timezone); tz != "" {
		if loaded, loadErr := time.LoadLocation(tz); loadErr == nil {
			location = loaded
		}
	}
	return fmt.Sprintf("Запись через календарь подтверждена: %s.", booking.StartsAt.In(location).Format("02.01 15:04")), nil
}
