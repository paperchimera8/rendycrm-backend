package telegram

import (
	"context"
	"errors"
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

func TestSendTextReturnsTelegramDescriptionOn400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`))
	}))
	defer server.Close()

	client := NewAPIClient(server.URL)
	_, err := client.SendText(context.Background(), "token", SendMessageRequest{
		ChatID: "1",
		Text:   "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status code: %+v", apiErr)
	}
	if apiErr.Description != "Bad Request: chat not found" {
		t.Fatalf("unexpected description: %+v", apiErr)
	}
	if apiErr.Retriable {
		t.Fatalf("unexpected retriable flag: %+v", apiErr)
	}
}

func TestSendTextRetriesOnTelegramErrorInside200Response(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := attempts.Add(1)
		w.WriteHeader(http.StatusOK)
		if current == 1 {
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":321}}`))
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
	if response.MessageID != 321 {
		t.Fatalf("unexpected message id: %+v", response)
	}
	if attempts.Load() < 2 {
		t.Fatalf("expected retry, got %d attempts", attempts.Load())
	}
}

func TestIsRetriableErrorRejectsGenericErrors(t *testing.T) {
	if IsRetriableError(errors.New("boom")) {
		t.Fatal("expected generic error to be non-retriable")
	}
}
