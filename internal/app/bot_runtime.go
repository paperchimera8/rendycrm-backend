package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const botRuntimeSecretHeader = "X-Bot-Runtime-Secret"

func (s *Server) shouldProxyTelegramWebhook(parts []string) bool {
	if s == nil {
		return false
	}
	if strings.TrimSpace(s.cfg.BotRuntimeBaseURL) == "" {
		return false
	}
	if len(parts) == 4 && parts[0] == "telegram" && parts[1] == "client" {
		return true
	}
	if len(parts) == 2 && parts[0] == "telegram" && parts[1] == "operator" {
		return true
	}
	return false
}

func (s *Server) proxyTelegramWebhookToBotRuntime(w http.ResponseWriter, r *http.Request) {
	if s == nil || strings.TrimSpace(s.cfg.BotRuntimeBaseURL) == "" {
		s.writeError(w, http.StatusBadGateway, "bot runtime proxy is not configured")
		return
	}
	if r == nil {
		s.writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid telegram update")
		return
	}
	_ = r.Body.Close()

	targetURL, err := resolveBotRuntimeProxyURL(s.cfg.BotRuntimeBaseURL, r.URL.Path, r.URL.RawQuery)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "invalid bot runtime url")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "failed to create bot runtime request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if value := strings.TrimSpace(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")); value != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", value)
	}
	if secret := strings.TrimSpace(s.cfg.BotRuntimeSecret); secret != "" {
		req.Header.Set(botRuntimeSecretHeader, secret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "bot runtime request failed")
		return
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "bot runtime response failed")
		return
	}

	if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

func resolveBotRuntimeProxyURL(baseURL, path, rawQuery string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", errors.New("empty bot runtime base url")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = rawQuery
	return parsed.String(), nil
}

func (s *Server) authorizeInternalBotRuntime(r *http.Request) bool {
	if s == nil {
		return false
	}
	expected := strings.TrimSpace(s.cfg.BotRuntimeSecret)
	if expected == "" {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get(botRuntimeSecretHeader))
	return provided != "" && provided == expected
}

func (s *Server) handleInternalTelegramOperatorWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authorizeInternalBotRuntime(r) {
		s.writeError(w, http.StatusUnauthorized, "invalid bot runtime secret")
		return
	}
	s.handleTelegramOperatorWebhook(w, r)
}

func (s *Server) handleInternalTelegramClientWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authorizeInternalBotRuntime(r) {
		s.writeError(w, http.StatusUnauthorized, "invalid bot runtime secret")
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/internal/bot-runtime/telegram/client/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		s.writeError(w, http.StatusNotFound, "bot runtime client webhook not found")
		return
	}
	s.handleTelegramClientWebhook(w, r, parts[0], parts[1])
}
