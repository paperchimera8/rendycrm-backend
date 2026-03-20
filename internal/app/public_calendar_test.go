package app

import (
	"testing"
	"time"
)

func TestPublicCalendarURL(t *testing.T) {
	t.Run("app base path is appended", func(t *testing.T) {
		link, err := publicCalendarURL("https://rendycrm.ru", "/app", "token-1")
		if err != nil {
			t.Fatalf("publicCalendarURL: %v", err)
		}
		want := "https://rendycrm.ru/app/calendar?token=token-1"
		if link != want {
			t.Fatalf("link mismatch: got %q want %q", link, want)
		}
	})

	t.Run("existing app path is not duplicated", func(t *testing.T) {
		link, err := publicCalendarURL("https://rendycrm.ru/app", "/app", "token-2")
		if err != nil {
			t.Fatalf("publicCalendarURL: %v", err)
		}
		want := "https://rendycrm.ru/app/calendar?token=token-2"
		if link != want {
			t.Fatalf("link mismatch: got %q want %q", link, want)
		}
	})

	t.Run("api suffix is stripped before calendar path is added", func(t *testing.T) {
		link, err := publicCalendarURL("https://rendycrm.ru/app/api", "/app", "token-3")
		if err != nil {
			t.Fatalf("publicCalendarURL: %v", err)
		}
		want := "https://rendycrm.ru/app/calendar?token=token-3"
		if link != want {
			t.Fatalf("link mismatch: got %q want %q", link, want)
		}
	})
}

func TestPublicCalendarAccessTokenRoundTrip(t *testing.T) {
	server := &Server{cfg: Config{EncryptionSecret: "test-secret"}}
	expiresAt := time.Now().UTC().Add(time.Hour).Round(time.Second)

	token, err := server.encodePublicCalendarAccessToken(publicCalendarAccessToken{
		WorkspaceID:      "ws_1",
		ChannelAccountID: "cha_1",
		ExternalChatID:   "chat_1",
		ExpiresAt:        expiresAt,
	})
	if err != nil {
		t.Fatalf("encodePublicCalendarAccessToken: %v", err)
	}

	payload, err := server.decodePublicCalendarAccessToken(token)
	if err != nil {
		t.Fatalf("decodePublicCalendarAccessToken: %v", err)
	}
	if payload.WorkspaceID != "ws_1" || payload.ChannelAccountID != "cha_1" || payload.ExternalChatID != "chat_1" {
		t.Fatalf("decoded payload mismatch: %+v", payload)
	}
	if !payload.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expiresAt mismatch: got %s want %s", payload.ExpiresAt, expiresAt)
	}
}

func TestPublicCalendarAccessTokenIsStableForSamePayload(t *testing.T) {
	server := &Server{cfg: Config{EncryptionSecret: "test-secret"}}
	payload := publicCalendarAccessToken{
		WorkspaceID:      "ws_1",
		ChannelAccountID: "cha_1",
		ExternalChatID:   "chat_1",
		ExpiresAt:        time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
	}

	first, err := server.encodePublicCalendarAccessToken(payload)
	if err != nil {
		t.Fatalf("first encode: %v", err)
	}
	second, err := server.encodePublicCalendarAccessToken(payload)
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable token, got %q and %q", first, second)
	}
}
