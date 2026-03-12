package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/vital/rendycrm-app/internal/domain"
	"github.com/vital/rendycrm-app/internal/usecase"
)

type AuthContext struct {
	Session   Session
	User      User
	Workspace Workspace
}

const sessionCookieName = "rendycrm_session"

type Server struct {
	cfg     Config
	runtime *Runtime
	mux     *http.ServeMux
}

func NewServer(ctx context.Context, cfg Config) (*Server, error) {
	runtime, err := NewRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	server := &Server{cfg: cfg, runtime: runtime, mux: http.NewServeMux()}
	server.routes()
	server.startWorker(ctx)
	return server, nil
}

func (s *Server) Close() error {
	return s.runtime.Close()
}

func (s *Server) Handler() http.Handler {
	return s.cors(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.mux.HandleFunc("/auth/login", s.handleLogin)
	s.mux.HandleFunc("/auth/logout", s.requireAuth(s.handleLogout))
	s.mux.HandleFunc("/auth/me", s.requireAuth(s.handleMe))
	s.mux.HandleFunc("/dashboard", s.requireAuth(s.handleDashboard))
	s.mux.HandleFunc("/conversations", s.requireAuth(s.handleConversations))
	s.mux.HandleFunc("/conversations/", s.requireAuth(s.handleConversationByID))
	s.mux.HandleFunc("/customers/", s.requireAuth(s.handleCustomerByID))
	s.mux.HandleFunc("/availability", s.requireAuth(s.handleAvailability))
	s.mux.HandleFunc("/availability/rules", s.requireAuth(s.handleAvailabilityRules))
	s.mux.HandleFunc("/availability/exceptions", s.requireAuth(s.handleAvailabilityExceptions))
	s.mux.HandleFunc("/slots/editor", s.requireAuth(s.handleSlotsEditor))
	s.mux.HandleFunc("/slots/week", s.requireAuth(s.handleWeekSlots))
	s.mux.HandleFunc("/slots/settings", s.requireAuth(s.handleSlotSettings))
	s.mux.HandleFunc("/slots/colors/reorder", s.requireAuth(s.handleSlotColorsReorder))
	s.mux.HandleFunc("/slots/colors", s.requireAuth(s.handleSlotColors))
	s.mux.HandleFunc("/slots/colors/", s.requireAuth(s.handleSlotColorByID))
	s.mux.HandleFunc("/slots/templates/reorder", s.requireAuth(s.handleSlotTemplatesReorder))
	s.mux.HandleFunc("/slots/templates", s.requireAuth(s.handleSlotTemplates))
	s.mux.HandleFunc("/slots/templates/", s.requireAuth(s.handleSlotTemplateByID))
	s.mux.HandleFunc("/slots/day-slots/move", s.requireAuth(s.handleDaySlotMove))
	s.mux.HandleFunc("/slots/day-slots/reorder", s.requireAuth(s.handleDaySlotsReorder))
	s.mux.HandleFunc("/slots/day-slots", s.requireAuth(s.handleDaySlots))
	s.mux.HandleFunc("/slots/day-slots/", s.requireAuth(s.handleDaySlotByID))
	s.mux.HandleFunc("/slots/available", s.requireAuth(s.handleAvailableSlots))
	s.mux.HandleFunc("/slots/", s.requireAuth(s.handleSlotByID))
	s.mux.HandleFunc("/slot-holds/", s.requireAuth(s.handleSlotHoldByID))
	s.mux.HandleFunc("/bookings", s.requireAuth(s.handleBookings))
	s.mux.HandleFunc("/bookings/", s.requireAuth(s.handleBookingByID))
	s.mux.HandleFunc("/reviews", s.requireAuth(s.handleReviews))
	s.mux.HandleFunc("/reviews/", s.requireAuth(s.handleReviewByID))
	s.mux.HandleFunc("/analytics/overview", s.requireAuth(s.handleAnalytics))
	s.mux.HandleFunc("/settings/channels", s.requireAuth(s.handleChannels))
	s.mux.HandleFunc("/settings/channels/", s.requireAuth(s.handleChannelByProvider))
	s.mux.HandleFunc("/settings/master-profile", s.requireAuth(s.handleMasterProfile))
	s.mux.HandleFunc("/settings/bot", s.requireAuth(s.handleBotConfig))
	s.mux.HandleFunc("/settings/operator-bot", s.requireAuth(s.handleOperatorBotSettings))
	s.mux.HandleFunc("/settings/operator-bot/", s.requireAuth(s.handleOperatorBotSettings))
	s.mux.HandleFunc("/events", s.requireAuth(s.handleEvents))
	s.mux.HandleFunc("/webhooks/", s.handleWebhook)
	s.mux.HandleFunc("/", s.handleApp)
}

func (s *Server) startWorker(ctx context.Context) {
	go s.processOutboundMessages(ctx)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("job worker panic: %v", recovered)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			job, err := s.runtime.jobs.Consume(ctx)
			if err != nil {
				log.Printf("job consume error: %v", err)
				time.Sleep(time.Second)
				continue
			}
			if job == nil {
				continue
			}
			switch job.Kind {
			case "analytics.refresh":
				var payload struct {
					WorkspaceID string `json:"workspaceId"`
				}
				if err := json.Unmarshal(job.Payload, &payload); err != nil || strings.TrimSpace(payload.WorkspaceID) == "" {
					log.Printf("analytics refresh skipped: invalid payload")
					continue
				}
				if err := s.runtime.repository.RefreshAnalytics(ctx, payload.WorkspaceID); err != nil {
					log.Printf("analytics refresh failed: %v", err)
				}
			default:
				log.Printf("ignoring unsupported job: %s", job.Kind)
			}
		}
	}()
}

func (s *Server) requireAuth(next func(http.ResponseWriter, *http.Request, AuthContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if token == "" {
			if cookie, err := r.Cookie(sessionCookieName); err == nil {
				token = strings.TrimSpace(cookie.Value)
			}
		}
		if token == "" {
			token = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		if token == "" {
			s.writeError(w, http.StatusUnauthorized, "missing token")
			return
		}
		ctx := r.Context()
		session, err := s.runtime.sessions.Get(ctx, token)
		if err != nil {
			if errors.Is(err, redis.Nil) {
				s.writeError(w, http.StatusUnauthorized, "invalid session")
				return
			}
			s.writeError(w, http.StatusInternalServerError, "session lookup failed")
			return
		}
		user, workspace, err := s.runtime.repository.Me(ctx, session.UserID, session.WorkspaceID)
		if err != nil {
			s.writeError(w, http.StatusUnauthorized, "invalid session")
			return
		}
		next(w, r, AuthContext{Session: session, User: user, Workspace: workspace})
	}
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && s.isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}

func (s *Server) decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (s *Server) handleApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeError(w, http.StatusNotFound, "not found")
		return
	}
	staticDir := strings.TrimSpace(s.cfg.StaticDir)
	if staticDir == "" {
		s.writeError(w, http.StatusNotFound, "not found")
		return
	}
	cleanPath := filepath.Clean("/" + strings.TrimSpace(r.URL.Path))
	targetPath := filepath.Join(staticDir, filepath.FromSlash(strings.TrimPrefix(cleanPath, "/")))
	if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
		if isStaticAssetRequest(cleanPath) {
			// Fingerprinted assets should be cached aggressively.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if isIndexPath(cleanPath) {
			// Never cache app shell html to avoid stale asset references after deploy.
			w.Header().Set("Cache-Control", "no-store")
		}
		http.ServeFile(w, r, targetPath)
		return
	}
	if isStaticAssetRequest(cleanPath) {
		s.writeError(w, http.StatusNotFound, "not found")
		return
	}
	indexPath := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		s.writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, indexPath)
}

func isStaticAssetRequest(path string) bool {
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/favicon") || strings.HasPrefix(path, "/manifest") {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".css", ".map", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico", ".woff", ".woff2", ".ttf", ".otf":
		return true
	default:
		return false
	}
}

func isIndexPath(path string) bool {
	return path == "/" || strings.EqualFold(path, "/index.html")
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, workspace, err := s.runtime.repository.Login(r.Context(), payload.Email, payload.Password)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	session := Session{
		Token:       newID("sess"),
		UserID:      user.ID,
		WorkspaceID: workspace.ID,
		ExpiresAt:   time.Now().UTC().Add(s.cfg.SessionTTL),
	}
	if err := s.runtime.sessions.Create(r.Context(), session); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	if err := s.runtime.repository.SaveSessionMetadata(r.Context(), session); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to persist session")
		return
	}
	s.setSessionCookie(w, r, session)
	s.writeJSON(w, http.StatusOK, map[string]any{"token": session.Token, "expiresAt": session.ExpiresAt, "user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.runtime.sessions.Delete(r.Context(), auth.Session.Token); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to clear session")
		return
	}
	_ = s.runtime.repository.DeleteSessionMetadata(r.Context(), auth.Session.Token)
	s.clearSessionCookie(w, r)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"user": auth.User, "workspace": auth.Workspace})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	dashboard, err := s.runtime.repository.Dashboard(r.Context(), auth.Workspace.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "dashboard query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, dashboard)
}

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.runtime.repository.Conversations(r.Context(), auth.Workspace.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "conversation query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleConversationByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	path := strings.TrimPrefix(r.URL.Path, "/conversations/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		s.writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	id := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		conversation, messages, customer, err := s.runtime.repository.ConversationDetail(r.Context(), auth.Workspace.ID, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation, "messages": messages, "customer": customer})
	case len(parts) == 2 && parts[1] == "reply" && r.Method == http.MethodPost:
		var payload struct {
			Text string `json:"text"`
		}
		if err := s.decodeJSON(r, &payload); err != nil || strings.TrimSpace(payload.Text) == "" {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		replyResult, err := s.runtime.services.Dialogs.ReplyToDialog(r.Context(), actor, id, payload.Text)
		if err != nil {
			status := http.StatusNotFound
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		conversation, messages, _, err := s.runtime.repository.ConversationDetail(r.Context(), auth.Workspace.ID, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		var message Message
		for _, item := range messages {
			if item.ID == replyResult.MessageID {
				message = item
				break
			}
		}
		_ = s.publishEvent(r.Context(), SSEEvent{Type: EventMessageNew, Data: message})
		_ = s.publishDashboard(r.Context(), auth.Workspace.ID)
		_ = s.runtime.jobs.Enqueue(r.Context(), "analytics.refresh", map[string]string{"workspaceId": auth.Workspace.ID})
		s.writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation, "message": message})
	case len(parts) == 2 && parts[1] == "assign" && r.Method == http.MethodPost:
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		err := s.runtime.services.Dialogs.TakeDialogByHuman(r.Context(), actor, id)
		if err != nil {
			status := http.StatusNotFound
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		conversation, _, _, err := s.runtime.repository.ConversationDetail(r.Context(), auth.Workspace.ID, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		_ = s.publishEvent(r.Context(), SSEEvent{Type: EventConversationAssign, Data: conversation})
		s.writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation})
	case len(parts) == 2 && parts[1] == "resolve" && r.Method == http.MethodPost:
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		err := s.runtime.services.Dialogs.CloseDialog(r.Context(), actor, id)
		if err != nil {
			status := http.StatusNotFound
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		conversation, _, _, err := s.runtime.repository.ConversationDetail(r.Context(), auth.Workspace.ID, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		_ = s.publishDashboard(r.Context(), auth.Workspace.ID)
		s.writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation})
	case len(parts) == 2 && parts[1] == "reopen" && r.Method == http.MethodPost:
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		err := s.runtime.services.Dialogs.ReopenDialog(r.Context(), actor, id)
		if err != nil {
			status := http.StatusNotFound
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		conversation, _, _, err := s.runtime.repository.ConversationDetail(r.Context(), auth.Workspace.ID, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		_ = s.publishDashboard(r.Context(), auth.Workspace.ID)
		s.writeJSON(w, http.StatusOK, map[string]any{"conversation": conversation})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleCustomerByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/customers/"), "/")
	switch r.Method {
	case http.MethodGet:
		customer, err := s.runtime.repository.Customer(r.Context(), auth.Workspace.ID, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "customer not found")
			return
		}
		s.writeJSON(w, http.StatusOK, customer)
	case http.MethodPatch:
		var payload struct {
			Name string `json:"name"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		_, err := s.runtime.services.Customers.UpdateProfile(r.Context(), actor, auth.Workspace.ID, id, payload.Name)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		customer, err := s.runtime.repository.Customer(r.Context(), auth.Workspace.ID, id)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		_ = s.publishDashboard(r.Context(), auth.Workspace.ID)
		s.writeJSON(w, http.StatusOK, customer)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAvailability(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rules, exceptions, _, _, err := s.runtime.repository.Availability(r.Context(), auth.Workspace.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "availability query failed")
		return
	}
	editor, err := s.runtime.repository.SlotEditor(r.Context(), auth.Workspace.ID, time.Now().UTC())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "availability query failed")
		return
	}
	available, err := s.runtime.repository.AvailableDaySlots(r.Context(), auth.Workspace.ID, time.Now().UTC(), time.Now().UTC().AddDate(0, 0, 14))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "availability query failed")
		return
	}
	legacySlots := make([]Slot, 0, len(available))
	for _, slot := range available {
		legacySlots = append(legacySlots, Slot{Start: slot.StartsAt, End: slot.EndsAt})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"rules": rules, "exceptions": exceptions, "slots": legacySlots, "editor": editor})
}

func (s *Server) handleAvailabilityRules(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Rules []AvailabilityRule `json:"rules"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	rules, err := s.runtime.repository.ReplaceAvailabilityRules(r.Context(), auth.Workspace.ID, payload.Rules)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to update rules")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (s *Server) handleAvailabilityExceptions(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Exceptions []AvailabilityException `json:"exceptions"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	exceptions, err := s.runtime.repository.ReplaceAvailabilityExceptions(r.Context(), auth.Workspace.ID, payload.Exceptions)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to update exceptions")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"exceptions": exceptions})
}

func (s *Server) handleBookings(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		items, err := s.runtime.repository.Bookings(r.Context(), auth.Workspace.ID, status)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "bookings query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var payload struct {
			CustomerID     string `json:"customerId"`
			DailySlotID    string `json:"dailySlotId"`
			StartsAt       string `json:"startsAt"`
			EndsAt         string `json:"endsAt"`
			Amount         int    `json:"amount"`
			Status         string `json:"status"`
			Notes          string `json:"notes"`
			ConversationID string `json:"conversationId"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		var startsAt, endsAt time.Time
		var parseErr error
		if payload.DailySlotID == "" {
			startsAt, parseErr = time.Parse(time.RFC3339, payload.StartsAt)
			if parseErr != nil {
				s.writeError(w, http.StatusBadRequest, "invalid startsAt")
				return
			}
			endsAt, parseErr = time.Parse(time.RFC3339, payload.EndsAt)
			if parseErr != nil {
				s.writeError(w, http.StatusBadRequest, "invalid endsAt")
				return
			}
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		result, err := s.runtime.services.Bookings.CreateBooking(r.Context(), actor, usecase.CreateBookingInput{
			WorkspaceID:    auth.Workspace.ID,
			CustomerID:     payload.CustomerID,
			DailySlotID:    payload.DailySlotID,
			StartsAt:       startsAt,
			EndsAt:         endsAt,
			Amount:         payload.Amount,
			Status:         payload.Status,
			Notes:          payload.Notes,
			ConversationID: payload.ConversationID,
		})
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		booking, err := s.runtime.repository.Booking(r.Context(), auth.Workspace.ID, result.ID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		_ = s.publishEvent(r.Context(), SSEEvent{Type: EventBookingUpdated, Data: booking})
		_ = s.publishDashboard(r.Context(), auth.Workspace.ID)
		_ = s.runtime.jobs.Enqueue(r.Context(), "analytics.refresh", map[string]string{"workspaceId": auth.Workspace.ID})
		s.writeJSON(w, http.StatusCreated, map[string]any{"booking": booking, "hold": SlotHold{}})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleBookingByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	path := strings.TrimPrefix(r.URL.Path, "/bookings/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var (
		booking Booking
		err     error
	)
	switch parts[1] {
	case "confirm":
		var payload struct {
			Amount         int    `json:"amount"`
			ConversationID string `json:"conversationId"`
		}
		if err := s.decodeJSON(r, &payload); err != nil && !errors.Is(err, io.EOF) {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		result, confirmErr := s.runtime.services.Bookings.ConfirmBooking(r.Context(), actor, auth.Workspace.ID, parts[0], payload.Amount, payload.ConversationID)
		err = confirmErr
		if err == nil {
			booking, err = s.runtime.repository.Booking(r.Context(), auth.Workspace.ID, result.ID)
		}
	case "complete":
		var payload struct {
			Amount int `json:"amount"`
		}
		if err := s.decodeJSON(r, &payload); err != nil && !errors.Is(err, io.EOF) {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		result, completeErr := s.runtime.services.Bookings.CompleteBooking(r.Context(), actor, auth.Workspace.ID, parts[0], payload.Amount)
		err = completeErr
		if err == nil {
			booking, err = s.runtime.repository.Booking(r.Context(), auth.Workspace.ID, result.ID)
		}
	case "cancel":
		var payload struct {
			ConversationID string `json:"conversationId"`
		}
		if err := s.decodeJSON(r, &payload); err != nil && !errors.Is(err, io.EOF) {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		result, cancelErr := s.runtime.services.Bookings.CancelBooking(r.Context(), actor, auth.Workspace.ID, parts[0], payload.ConversationID)
		err = cancelErr
		if err == nil {
			booking, err = s.runtime.repository.Booking(r.Context(), auth.Workspace.ID, result.ID)
		}
	case "reschedule":
		var payload struct {
			DailySlotID    string `json:"dailySlotId"`
			StartsAt       string `json:"startsAt"`
			EndsAt         string `json:"endsAt"`
			Amount         int    `json:"amount"`
			Status         string `json:"status"`
			Notes          string `json:"notes"`
			ConversationID string `json:"conversationId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		var startsAt, endsAt time.Time
		if payload.DailySlotID == "" {
			startsAt, err = time.Parse(time.RFC3339, payload.StartsAt)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, "invalid startsAt")
				return
			}
			endsAt, err = time.Parse(time.RFC3339, payload.EndsAt)
			if err != nil {
				s.writeError(w, http.StatusBadRequest, "invalid endsAt")
				return
			}
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		result, rescheduleErr := s.runtime.services.Bookings.RescheduleBooking(r.Context(), actor, usecase.UpdateBookingInput{
			WorkspaceID:    auth.Workspace.ID,
			BookingID:      parts[0],
			DailySlotID:    payload.DailySlotID,
			StartsAt:       startsAt,
			EndsAt:         endsAt,
			Amount:         payload.Amount,
			Status:         payload.Status,
			Notes:          payload.Notes,
			ConversationID: payload.ConversationID,
		})
		err = rescheduleErr
		if err == nil {
			booking, err = s.runtime.repository.Booking(r.Context(), auth.Workspace.ID, result.ID)
		}
	default:
		s.writeError(w, http.StatusNotFound, "action not found")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	_ = s.publishEvent(r.Context(), SSEEvent{Type: EventBookingUpdated, Data: booking})
	_ = s.publishDashboard(r.Context(), auth.Workspace.ID)
	_ = s.runtime.jobs.Enqueue(r.Context(), "analytics.refresh", map[string]string{"workspaceId": auth.Workspace.ID})
	s.writeJSON(w, http.StatusOK, map[string]any{"booking": booking})
}

func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.runtime.repository.Reviews(r.Context(), auth.Workspace.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "reviews query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleReviewByID(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	path := strings.TrimPrefix(r.URL.Path, "/reviews/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "status" || r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Status ReviewStatus `json:"status"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
	_, err := s.runtime.services.Reviews.UpdateStatus(r.Context(), actor, auth.Workspace.ID, parts[0], string(payload.Status))
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, domain.ErrAccessDenied) {
			status = http.StatusForbidden
		}
		s.writeError(w, status, err.Error())
		return
	}
	review, err := s.runtime.repository.Review(r.Context(), auth.Workspace.ID, parts[0])
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	_ = s.publishEvent(r.Context(), SSEEvent{Type: EventReviewNew, Data: review})
	_ = s.publishDashboard(r.Context(), auth.Workspace.ID)
	s.writeJSON(w, http.StatusOK, map[string]any{"review": review})
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.runtime.repository.RefreshAnalytics(r.Context(), auth.Workspace.ID); err != nil {
		s.writeError(w, http.StatusInternalServerError, "analytics refresh failed")
		return
	}
	overview, err := s.runtime.repository.Analytics(r.Context(), auth.Workspace.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "analytics query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.runtime.repository.ChannelAccounts(r.Context(), auth.Workspace.ID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "channel query failed")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleMasterProfile(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	switch r.Method {
	case http.MethodGet:
		profile, err := s.runtime.repository.MasterProfile(r.Context(), auth.Workspace.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "master profile query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, profile)
	case http.MethodPut:
		var payload struct {
			MasterPhone string `json:"masterPhone"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		_, err := s.runtime.services.Channels.UpdateMasterProfile(r.Context(), actor, auth.Workspace.ID, payload.MasterPhone)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		profile, err := s.runtime.repository.MasterProfile(r.Context(), auth.Workspace.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "master profile query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, profile)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleChannelByProvider(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	provider := ChannelProvider(strings.Trim(strings.TrimPrefix(r.URL.Path, "/settings/channels/"), "/"))
	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Connected     bool   `json:"connected"`
		Name          string `json:"name"`
		BotUsername   string `json:"botUsername"`
		BotToken      string `json:"botToken"`
		WebhookSecret string `json:"webhookSecret"`
	}
	if err := s.decodeJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if provider == ChannelTelegram && payload.Connected {
		profile, err := s.runtime.repository.MasterProfile(r.Context(), auth.Workspace.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "master profile query failed")
			return
		}
		if strings.TrimSpace(profile.MasterPhoneNormalized) == "" {
			s.writeError(w, http.StatusBadRequest, "сначала сохраните номер мастера в настройках")
			return
		}
	}
	if payload.BotToken == "" && payload.BotUsername == "" && payload.WebhookSecret == "" {
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		result, err := s.runtime.services.Channels.SaveChannelSettings(r.Context(), actor, usecase.ChannelAccountInput{
			WorkspaceID:  auth.Workspace.ID,
			Provider:     string(provider),
			ChannelKind:  string(defaultChannelKind(provider)),
			AccountScope: string(ChannelAccountScopeWorkspace),
			Name:         payload.Name,
			Connected:    payload.Connected,
			IsEnabled:    payload.Connected,
		})
		if err != nil {
			status := http.StatusNotFound
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, err.Error())
			return
		}
		account, err := s.runtime.repository.ChannelAccountByID(r.Context(), result.ID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.writeJSON(w, http.StatusOK, account)
		return
	}
	encryptedToken := ""
	if strings.TrimSpace(payload.BotToken) != "" {
		var err error
		encryptedToken, err = encryptString(s.cfg.EncryptionSecret, payload.BotToken)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "failed to encrypt bot token")
			return
		}
	}
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
	result, err := s.runtime.services.Channels.SaveChannelSettings(r.Context(), actor, usecase.ChannelAccountInput{
		WorkspaceID:     auth.Workspace.ID,
		Provider:        string(provider),
		ChannelKind:     string(defaultChannelKind(provider)),
		AccountScope:    string(ChannelAccountScopeWorkspace),
		Name:            payload.Name,
		Connected:       payload.Connected,
		IsEnabled:       payload.Connected,
		BotUsername:     strings.TrimSpace(payload.BotUsername),
		WebhookSecret:   strings.TrimSpace(payload.WebhookSecret),
		EncryptedToken:  encryptedToken,
		TokenConfigured: encryptedToken != "",
	})
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, domain.ErrAccessDenied) {
			status = http.StatusForbidden
		}
		s.writeError(w, status, err.Error())
		return
	}
	account, err := s.runtime.repository.ChannelAccountByID(r.Context(), result.ID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, account)
}

func (s *Server) handleBotConfig(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	switch r.Method {
	case http.MethodGet:
		config, faq, err := s.runtime.repository.BotConfig(r.Context(), auth.Workspace.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "bot config query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"config": config, "faqItems": faq})
	case http.MethodPut:
		var payload struct {
			Config   BotConfig `json:"config"`
			FAQItems []FAQItem `json:"faqItems"`
		}
		if err := s.decodeJSON(r, &payload); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid payload")
			return
		}
		actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: auth.Workspace.ID, UserID: auth.User.ID, Role: string(auth.User.Role)}
		_, _, err := s.runtime.services.BotSettings.UpdateConfig(r.Context(), actor, auth.Workspace.ID, usecase.BotConfigState{
			AutoReply:      payload.Config.AutoReply,
			HandoffEnabled: payload.Config.HandoffEnabled,
			Tone:           payload.Config.Tone,
			WelcomeMessage: payload.Config.WelcomeMessage,
			HandoffMessage: payload.Config.HandoffMessage,
		}, func(items []FAQItem) []usecase.FAQItem {
			result := make([]usecase.FAQItem, 0, len(items))
			for _, item := range items {
				result = append(result, usecase.FAQItem{ID: item.ID, Question: item.Question, Answer: item.Answer})
			}
			return result
		}(payload.FAQItems))
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrAccessDenied) {
				status = http.StatusForbidden
			}
			s.writeError(w, status, "failed to update bot config")
			return
		}
		config, faq, err := s.runtime.repository.BotConfig(r.Context(), auth.Workspace.ID)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "bot config query failed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"config": config, "faqItems": faq})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, _ AuthContext) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	pubsub := s.runtime.events.Subscribe(r.Context())
	defer pubsub.Close()
	fmt.Fprintf(w, "event: ready\ndata: {\"ok\":true}\n\n")
	flusher.Flush()
	ch := pubsub.Channel()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case msg := <-ch:
			var event SSEEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				continue
			}
			payload, _ := json.Marshal(event.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payload)
			flusher.Flush()
		}
	}
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/webhooks/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		s.writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	switch {
	case len(parts) == 4 && parts[0] == "telegram" && parts[1] == "client":
		s.handleTelegramClientWebhook(w, r, parts[2], parts[3])
	case len(parts) == 4 && parts[0] == "whatsapp" && parts[1] == "twilio":
		s.handleWhatsAppWebhook(w, r, parts[2], parts[3])
	case len(parts) == 2 && parts[0] == "telegram" && parts[1] == "operator":
		s.handleTelegramOperatorWebhook(w, r)
	default:
		s.writeError(w, http.StatusNotFound, "webhook not found")
	}
}

func (s *Server) publishDashboard(ctx context.Context, workspaceID string) error {
	dashboard, err := s.runtime.repository.Dashboard(ctx, workspaceID)
	if err != nil {
		return err
	}
	return s.publishEvent(ctx, SSEEvent{Type: EventDashboardUpdated, Data: dashboard})
}

func (s *Server) publishEvent(ctx context.Context, event SSEEvent) error {
	return s.runtime.events.Publish(ctx, event)
}

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}

func (s *Server) isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	for _, allowed := range s.cfg.CORSAllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	if s.cfg.PublicBaseURL != "" {
		if publicURL, err := url.Parse(s.cfg.PublicBaseURL); err == nil && publicURL.Scheme != "" && publicURL.Host != "" {
			return origin == publicURL.Scheme+"://"+publicURL.Host
		}
	}
	return false
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, session Session) {
	maxAge := int(time.Until(session.ExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		Expires:  session.ExpiresAt,
		MaxAge:   maxAge,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsSecure(r),
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
