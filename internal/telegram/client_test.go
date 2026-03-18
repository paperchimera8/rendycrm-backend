package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSendTextRetriesOn429(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := attempts.Add(1)
		if current == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":123}}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	response, err := client.SendText(context.Background(), "token", SendMessageRequest{
		ChatID: "1",
		Text:   "hello",
	})
	if err != nil {
		t.Fatalf("send text: %v", err)
	}
	if response.MessageID != 123 {
		t.Fatalf("unexpected message id: %+v", response)
	}
	if attempts.Load() < 2 {
		t.Fatalf("expected retry, got %d attempts", attempts.Load())
	}
}

func TestSetWebhookSendsSecretAndURL(t *testing.T) {
	var payload SetWebhookRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/setWebhook" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	err := client.SetWebhook(context.Background(), "token", SetWebhookRequest{
		URL:            "https://example.com/api/webhooks/telegram/operator",
		SecretToken:    "secret-1",
		AllowedUpdates: []string{"message", "callback_query"},
	})
	if err != nil {
		t.Fatalf("set webhook: %v", err)
	}
	if payload.URL != "https://example.com/api/webhooks/telegram/operator" {
		t.Fatalf("unexpected webhook url: %+v", payload)
	}
	if payload.SecretToken != "secret-1" {
		t.Fatalf("unexpected secret token: %+v", payload)
	}
	if len(payload.AllowedUpdates) != 2 {
		t.Fatalf("unexpected allowed updates: %+v", payload)
	}
}

func TestDeleteWebhookSendsRequest(t *testing.T) {
	var payload DeleteWebhookRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/deleteWebhook" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	if err := client.DeleteWebhook(context.Background(), "token", true); err != nil {
		t.Fatalf("delete webhook: %v", err)
	}
	if !payload.DropPendingUpdates {
		t.Fatalf("expected drop pending updates flag")
	}
}
