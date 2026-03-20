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

func TestParsePublicCalendarRange(t *testing.T) {
	now := time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC)

	t.Run("date_to is treated as inclusive last day", func(t *testing.T) {
		from, to, err := parsePublicCalendarRange(now, "2026-03-20", "2026-03-22")
		if err != nil {
			t.Fatalf("parsePublicCalendarRange: %v", err)
		}
		if want := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC); !from.Equal(want) {
			t.Fatalf("unexpected from: got %s want %s", from, want)
		}
		if want := time.Date(2026, 3, 23, 0, 0, 0, 0, time.UTC); !to.Equal(want) {
			t.Fatalf("unexpected to: got %s want %s", to, want)
		}
	})

	t.Run("date_from without date_to uses 14-day window from requested day", func(t *testing.T) {
		from, to, err := parsePublicCalendarRange(now, "2026-03-25", "")
		if err != nil {
			t.Fatalf("parsePublicCalendarRange: %v", err)
		}
		if want := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC); !from.Equal(want) {
			t.Fatalf("unexpected from: got %s want %s", from, want)
		}
		if want := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC); !to.Equal(want) {
			t.Fatalf("unexpected to: got %s want %s", to, want)
		}
	})
}
