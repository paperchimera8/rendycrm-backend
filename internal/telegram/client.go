package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

type APIError struct {
	Method      string
	StatusCode  int
	Description string
	RetryAfter  time.Duration
	Retriable   bool
}

func (e *APIError) Error() string {
	if e == nil {
		return "telegram api error"
	}
	switch {
	case e.StatusCode > 0 && e.Description != "":
		return fmt.Sprintf("telegram %s failed with status %d: %s", e.Method, e.StatusCode, e.Description)
	case e.StatusCode > 0:
		return fmt.Sprintf("telegram %s failed with status %d", e.Method, e.StatusCode)
	case e.Description != "":
		return fmt.Sprintf("telegram %s failed: %s", e.Method, e.Description)
	default:
		return fmt.Sprintf("telegram %s failed", e.Method)
	}
}

func IsRetriableError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retriable
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return !errors.Is(err, context.Canceled)
}

func NewAPIClient(baseURL string) *APIClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *APIClient) SendText(ctx context.Context, token string, req SendMessageRequest) (MessageResponse, error) {
	return c.callMessage(ctx, token, "sendMessage", req)
}

func (c *APIClient) SendInline(ctx context.Context, token string, chatID, text string, buttons [][]InlineKeyboardButton) (MessageResponse, error) {
	return c.callMessage(ctx, token, "sendMessage", SendMessageRequest{
		ChatID: chatID,
		Text:   text,
		ReplyMarkup: InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		},
	})
}

func (c *APIClient) EditInline(ctx context.Context, token string, req EditMessageTextRequest) (MessageResponse, error) {
	return c.callMessage(ctx, token, "editMessageText", req)
}

func (c *APIClient) AnswerCallback(ctx context.Context, token string, req AnswerCallbackQueryRequest) error {
	_, err := callAPI[map[string]any](ctx, c.httpClient, c.baseURL, token, "answerCallbackQuery", req)
	return err
}

func (c *APIClient) SetWebhook(ctx context.Context, token string, req SetWebhookRequest) error {
	_, err := callAPI[bool](ctx, c.httpClient, c.baseURL, token, "setWebhook", req)
	return err
}

func (c *APIClient) DeleteWebhook(ctx context.Context, token string, dropPendingUpdates bool) error {
	_, err := callAPI[bool](ctx, c.httpClient, c.baseURL, token, "deleteWebhook", DeleteWebhookRequest{
		DropPendingUpdates: dropPendingUpdates,
	})
	return err
}

func (c *APIClient) callMessage(ctx context.Context, token, method string, payload any) (MessageResponse, error) {
	return callAPI[MessageResponse](ctx, c.httpClient, c.baseURL, token, method, payload)
}

func callAPI[T any](ctx context.Context, httpClient *http.Client, baseURL, token, method string, payload any) (T, error) {
	var zero T
	if strings.TrimSpace(token) == "" {
		return zero, fmt.Errorf("telegram token is empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return zero, err
	}
	endpoint := fmt.Sprintf("%s/bot%s/%s", baseURL, url.PathEscape(token), method)

	var lastErr error
	delay := 300 * time.Millisecond
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return zero, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			rawBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			if readErr != nil {
				return zero, readErr
			}

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				var parsed APIResponse[T]
				if err := json.Unmarshal(rawBody, &parsed); err != nil {
					return zero, err
				}
				if !parsed.OK {
					lastErr = &APIError{
						Method:      method,
						Description: parsed.Description,
						Retriable:   false,
					}
				} else {
					return parsed.Result, nil
				}
			} else {
				retryAfter := parseRetryAfter(resp, rawBody)
				log.Printf("telegram api error method=%s status=%d retry_after=%s", method, resp.StatusCode, retryAfter)
				if shouldRetryStatus(resp.StatusCode) && attempt < 3 {
					if retryAfter > 0 {
						delay = retryAfter
					}
					select {
					case <-ctx.Done():
						return zero, ctx.Err()
					case <-time.After(delay):
					}
					delay *= 2
					continue
				}
				lastErr = &APIError{
					Method:      method,
					StatusCode:  resp.StatusCode,
					Description: strings.TrimSpace(http.StatusText(resp.StatusCode)),
					RetryAfter:  retryAfter,
					Retriable:   shouldRetryStatus(resp.StatusCode),
				}
			}
		}
		if attempt < 3 && shouldRetryError(lastErr) {
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			continue
		}
		break
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("telegram %s failed", method)
	}
	return zero, lastErr
}

func shouldRetryStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func shouldRetryError(err error) bool {
	return IsRetriableError(err)
}

func parseRetryAfter(resp *http.Response, body []byte) time.Duration {
	if header := strings.TrimSpace(resp.Header.Get("Retry-After")); header != "" {
		if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	var parsed APIResponse[map[string]any]
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Parameters.RetryAfter > 0 {
		return time.Duration(parsed.Parameters.RetryAfter) * time.Second
	}
	return 0
}
