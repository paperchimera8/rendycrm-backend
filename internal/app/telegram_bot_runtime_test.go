package app

import (
	"strings"
	"testing"
)

func TestTelegramOutboundDedupIdentityUsesMessageIDWhenCallbackMissing(t *testing.T) {
	got := telegramOutboundDedupIdentity(178, "")
	if got != "msg:178" {
		t.Fatalf("expected message identity, got %q", got)
	}
}

func TestTelegramOutboundDedupIdentityUsesCallbackIDHash(t *testing.T) {
	first := telegramOutboundDedupIdentity(178, "cbq_1")
	second := telegramOutboundDedupIdentity(178, "cbq_1")
	third := telegramOutboundDedupIdentity(178, "cbq_2")

	if first == "" {
		t.Fatal("expected callback identity")
	}
	if first != second {
		t.Fatalf("expected stable callback identity, got %q and %q", first, second)
	}
	if first == third {
		t.Fatalf("expected different callback ids to produce different identities, got %q", first)
	}
}

func TestTelegramOutboundDedupKeyChangesWithInboundIdentity(t *testing.T) {
	account := ChannelAccount{
		ID:          "cha_global_tg",
		ChannelKind: ChannelKindTelegramClient,
	}
	payload := TelegramOutboundPayload{
		ChatID: "1348661149",
		Text:   "Мастер выбран: Smoke Workspace.",
		Buttons: []TelegramInlineButton{
			{Text: "Свободные окна", CallbackData: "client:slots"},
		},
	}

	first := telegramOutboundDedupKey(account, payload.ChatID, 178, "", OutboundKindTelegramSendInline, "conv_1", "", payload)
	duplicate := telegramOutboundDedupKey(account, payload.ChatID, 178, "", OutboundKindTelegramSendInline, "conv_1", "", payload)
	second := telegramOutboundDedupKey(account, payload.ChatID, 179, "", OutboundKindTelegramSendInline, "conv_1", "", payload)

	if first == "" {
		t.Fatal("expected dedup key")
	}
	if first != duplicate {
		t.Fatalf("expected stable dedup key, got %q and %q", first, duplicate)
	}
	if first == second {
		t.Fatalf("expected different inbound messages to have different dedup keys, got %q", first)
	}
}

func TestBuildBotEngineReplyButtonsUsesCalendarURL(t *testing.T) {
	server := &Server{
		cfg: Config{
			PublicBaseURL:    "https://rendycrm.ru",
			AppBasePath:      "/app",
			EncryptionSecret: "test-secret",
		},
	}
	account := ChannelAccount{
		ID:          "cha_global_tg",
		WorkspaceID: "ws_smoke",
		ChannelKind: ChannelKindTelegramClient,
	}
	state := botEngineClientSession{
		Route: botEngineClientRouteState{
			Kind:        "ready",
			WorkspaceID: "ws_smoke",
		},
	}

	buttons := server.buildBotEngineReplyButtons(account, "1348661149", &state, []botEngineButton{
		{Text: "Записаться", Action: "client:open_calendar"},
	})
	if len(buttons) != 1 {
		t.Fatalf("expected one button, got %d", len(buttons))
	}
	if buttons[0].CallbackData != "" {
		t.Fatalf("expected url button, got callback %q", buttons[0].CallbackData)
	}
	if !strings.HasPrefix(buttons[0].URL, "https://rendycrm.ru/app/calendar?token=") {
		t.Fatalf("unexpected calendar url: %q", buttons[0].URL)
	}
}
