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

const (
	apiCallMaxAttempts    = 4
	apiRetryInitialDelay  = 300 * time.Millisecond
	apiResponseBodyMaxLen = 1 << 20
)

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
	if errors.Is(err, context.Canceled) {
		return false
	}
	// The request reached the server — we cannot know if it was processed.
	// Retrying would risk sending the same message twice.
	var ste *sentToServerErr
	if errors.As(err, &ste) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retriable
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return false
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
	delay := apiRetryInitialDelay
	for attempt := 0; attempt < apiCallMaxAttempts; attempt++ {
		result, err := doAPICallAttempt[T](ctx, httpClient, endpoint, method, body)
		if err != nil {
			lastErr = err
		} else {
			return result, nil
		}

		var ste *sentToServerErr
		if !errors.As(lastErr, &ste) && attempt < apiCallMaxAttempts-1 && shouldRetryError(lastErr) {
			delay = retryDelayFromError(delay, lastErr)
			if err := waitRetry(ctx, delay); err != nil {
				return zero, err
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

// sentToServerErr wraps an error that occurred after the HTTP request was
// received by the server (e.g. a body-read timeout). callAPI does not retry
// these errors to avoid sending the same message more than once.
type sentToServerErr struct{ cause error }

func (e *sentToServerErr) Error() string { return e.cause.Error() }
func (e *sentToServerErr) Unwrap() error { return e.cause }

func doAPICallAttempt[T any](ctx context.Context, httpClient *http.Client, endpoint, method string, body []byte) (T, error) {
	var zero T
	req, err := newJSONRequest(ctx, endpoint, body)
	if err != nil {
		return zero, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, err
	}

	rawBody, err := readResponseBody(resp)
	if err != nil {
		// The request reached the server — wrap the error so callAPI does not
		// retry it. Retrying after a body-read failure risks sending duplicates
		// because we cannot know whether Telegram already processed the request.
		return zero, &sentToServerErr{cause: err}
	}

	return parseAPIResponse[T](method, resp, rawBody)
}

func newJSONRequest(ctx context.Context, endpoint string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, apiResponseBodyMaxLen))
}

func parseAPIResponse[T any](method string, resp *http.Response, rawBody []byte) (T, error) {
	var zero T
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var parsed APIResponse[T]
		if err := json.Unmarshal(rawBody, &parsed); err != nil {
			return zero, err
		}
		if parsed.OK {
			return parsed.Result, nil
		}
		return zero, apiErrorFromResponse(method, resp.StatusCode, parseRetryAfterBody(rawBody), rawBody)
	}

	apiErr := apiErrorFromResponse(method, resp.StatusCode, parseRetryAfter(resp, rawBody), rawBody)
	log.Printf("telegram api error method=%s status=%d retry_after=%s description=%q", method, apiErr.StatusCode, apiErr.RetryAfter, apiErr.Description)
	return zero, apiErr
}

func retryDelayFromError(current time.Duration, err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return apiErr.RetryAfter
	}
	return current
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
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
	return parseRetryAfterBody(body)
}

func parseRetryAfterBody(body []byte) time.Duration {
	var parsed APIResponse[map[string]any]
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Parameters.RetryAfter > 0 {
		return time.Duration(parsed.Parameters.RetryAfter) * time.Second
	}
	return 0
}

func apiErrorFromResponse(method string, statusCode int, retryAfter time.Duration, body []byte) *APIError {
	apiErr := &APIError{
		Method:      method,
		StatusCode:  statusCode,
		Description: strings.TrimSpace(http.StatusText(statusCode)),
		RetryAfter:  retryAfter,
		Retriable:   shouldRetryStatus(statusCode),
	}
	var parsed APIResponse[json.RawMessage]
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.ErrorCode > 0 {
			apiErr.StatusCode = parsed.ErrorCode
			apiErr.Retriable = shouldRetryStatus(parsed.ErrorCode)
		}
		if value := strings.TrimSpace(parsed.Description); value != "" {
			apiErr.Description = value
		}
		if parsed.Parameters.RetryAfter > 0 {
			apiErr.RetryAfter = time.Duration(parsed.Parameters.RetryAfter) * time.Second
		}
	}
	return apiErr
}
