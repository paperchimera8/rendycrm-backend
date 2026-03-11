package telegram

import (
	"context"
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
