package app

import (
	"strings"
	"testing"

	tgapi "github.com/vital/rendycrm-app/internal/telegram"
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

	first := telegramOutboundDedupKey(account, payload.ChatID, 178, "", "", OutboundKindTelegramSendInline, "conv_1", "", payload)
	duplicate := telegramOutboundDedupKey(account, payload.ChatID, 178, "", "", OutboundKindTelegramSendInline, "conv_1", "", payload)
	second := telegramOutboundDedupKey(account, payload.ChatID, 179, "", "", OutboundKindTelegramSendInline, "conv_1", "", payload)

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

func TestTelegramOutboundDedupKeyUsesCallbackIDForCallbacks(t *testing.T) {
	account := ChannelAccount{
		ID:          "cha_global_tg",
		ChannelKind: ChannelKindTelegramOperator,
	}
	payload := TelegramOutboundPayload{
		ChatID: "1348661149",
		Text:   "Напоминания обновлены.",
	}

	first := telegramOutboundDedupKey(account, payload.ChatID, 412, "cbq_1", "rmd:toggle:booking_1:on", OutboundKindTelegramSendText, "", "", payload)
	duplicate := telegramOutboundDedupKey(account, payload.ChatID, 412, "cbq_1", "rmd:toggle:booking_1:on", OutboundKindTelegramSendText, "", "", payload)
	second := telegramOutboundDedupKey(account, payload.ChatID, 412, "cbq_2", "rmd:toggle:booking_1:on", OutboundKindTelegramSendText, "", "", payload)

	if first == "" {
		t.Fatal("expected dedup key")
	}
	if first != duplicate {
		t.Fatalf("expected stable callback dedup key, got %q and %q", first, duplicate)
	}
	if first == second {
		t.Fatalf("expected different callback ids to produce different dedup keys, got %q", first)
	}
}

func TestTelegramOperatorReplyKeyStableForSameReply(t *testing.T) {
	payload := TelegramOutboundPayload{
		ChatID: "1348661149",
		Text:   "Напоминания",
		Buttons: []TelegramInlineButton{
			{Text: "Вкл", CallbackData: "rmd:toggle:booking_1:on"},
		},
	}

	first := telegramOperatorReplyKey("cha_global_tg", payload.ChatID, 412, "rmd:toggle:booking_1:on", payload)
	second := telegramOperatorReplyKey("cha_global_tg", payload.ChatID, 412, "rmd:toggle:booking_1:on", payload)

	if first == "" {
		t.Fatal("expected operator reply key")
	}
	if first != second {
		t.Fatalf("expected stable operator reply key, got %q and %q", first, second)
	}
}

func TestTelegramOperatorReplyKeyDiffersForDifferentReplies(t *testing.T) {
	first := telegramOperatorReplyKey("cha_global_tg", "1348661149", 412, "rmd:toggle:booking_1:on", TelegramOutboundPayload{
		ChatID: "1348661149",
		Text:   "Напоминания",
		Buttons: []TelegramInlineButton{
			{Text: "Вкл", CallbackData: "rmd:toggle:booking_1:on"},
		},
	})
	second := telegramOperatorReplyKey("cha_global_tg", "1348661149", 412, "rmd:toggle:booking_1:off", TelegramOutboundPayload{
		ChatID: "1348661149",
		Text:   "Напоминания",
		Buttons: []TelegramInlineButton{
			{Text: "Выкл", CallbackData: "rmd:toggle:booking_1:off"},
		},
	})

	if first == second {
		t.Fatalf("expected different replies to produce different operator reply keys, got %q", first)
	}
}

func TestTelegramOperatorReplyKeyUsesSourceMessageAndAction(t *testing.T) {
	payload := TelegramOutboundPayload{
		ChatID: "1348661149",
		Text:   "Напоминания обновлены.",
	}

	first := telegramOperatorReplyKey("cha_global_tg", payload.ChatID, 412, "rmd:toggle:booking_1:on", payload)
	second := telegramOperatorReplyKey("cha_global_tg", payload.ChatID, 412, "rmd:toggle:booking_1:on", payload)
	third := telegramOperatorReplyKey("cha_global_tg", payload.ChatID, 412, "rmd:toggle:booking_1:off", payload)
	fourth := telegramOperatorReplyKey("cha_global_tg", payload.ChatID, 413, "rmd:toggle:booking_1:on", payload)

	if first != second {
		t.Fatalf("expected same source action to reuse reply key, got %q and %q", first, second)
	}
	if first == third {
		t.Fatalf("expected different actions to produce different reply keys, got %q", first)
	}
	if first == fourth {
		t.Fatalf("expected different source messages to produce different reply keys, got %q", first)
	}
}

func TestBotEngineReplyOutboundKindEditsOperatorCallbackReplies(t *testing.T) {
	account := ChannelAccount{
		ID:          "cha_operator_tg",
		ChannelKind: ChannelKindTelegramOperator,
	}

	got := botEngineReplyOutboundKind(account, 175, "cbq-1", []TelegramInlineButton{
		{Text: "Дашборд", CallbackData: "/dashboard"},
	})

	if got != OutboundKindTelegramEditInline {
		t.Fatalf("expected operator callback reply to use edit inline, got %s", got)
	}
}

func TestBotEngineReplyOutboundKindKeepsOperatorMessagesAsNewMessages(t *testing.T) {
	account := ChannelAccount{
		ID:          "cha_operator_tg",
		ChannelKind: ChannelKindTelegramOperator,
	}

	got := botEngineReplyOutboundKind(account, 175, "", []TelegramInlineButton{
		{Text: "Дашборд", CallbackData: "/dashboard"},
	})

	if got != OutboundKindTelegramSendInline {
		t.Fatalf("expected operator non-callback reply to send inline, got %s", got)
	}
}

func TestBotEngineReplyOutboundKindKeepsClientCallbacksAsInlineMessages(t *testing.T) {
	account := ChannelAccount{
		ID:          "cha_client_tg",
		ChannelKind: ChannelKindTelegramClient,
	}

	got := botEngineReplyOutboundKind(account, 287, "cbq-1", []TelegramInlineButton{
		{Text: "Ввести номер мастера", CallbackData: "client:enter_master_phone"},
	})

	if got != OutboundKindTelegramSendInline {
		t.Fatalf("expected client callback reply to keep send inline, got %s", got)
	}
}

func TestOperatorCommandNeedsThrottle(t *testing.T) {
	if !operatorCommandNeedsThrottle("/start") {
		t.Fatal("expected /start to be throttled")
	}
	if !operatorCommandNeedsThrottle(" /reminders ") {
		t.Fatal("expected slash command with whitespace to be throttled")
	}
	if !operatorCommandNeedsThrottle("отмена") {
		t.Fatal("expected отмена to be throttled")
	}
	if operatorCommandNeedsThrottle("спасибо") {
		t.Fatal("did not expect plain text to be throttled")
	}
}

func TestTelegramOperatorCommandKeyStable(t *testing.T) {
	first := telegramOperatorCommandKey("cha_global_tg", "1348661149", "/start")
	second := telegramOperatorCommandKey("cha_global_tg", "1348661149", " /start ")
	third := telegramOperatorCommandKey("cha_global_tg", "1348661149", "/reminders")

	if first == "" {
		t.Fatal("expected operator command key")
	}
	if first != second {
		t.Fatalf("expected stable operator command key, got %q and %q", first, second)
	}
	if first == third {
		t.Fatalf("expected different commands to produce different keys, got %q", first)
	}
}

func TestTelegramOperatorCommandForUpdateUsesCallbackMenuCommands(t *testing.T) {
	update := tgapi.Update{
		CallbackQuery: &tgapi.CallbackQuery{
			Data: "/dashboard",
			Message: &tgapi.Message{
				MessageID: 42,
				Chat:      tgapi.Chat{ID: 1348661149},
			},
		},
	}

	if got := telegramOperatorCommandForUpdate(update, "/dashboard"); got != "/dashboard" {
		t.Fatalf("expected callback dashboard command to be throttled, got %q", got)
	}
}

func TestTelegramOperatorCommandForUpdateSkipsNonMenuCallbacks(t *testing.T) {
	update := tgapi.Update{
		CallbackQuery: &tgapi.CallbackQuery{
			Data: "reminder:toggle:bok_1:on",
			Message: &tgapi.Message{
				MessageID: 42,
				Chat:      tgapi.Chat{ID: 1348661149},
			},
		},
	}

	if got := telegramOperatorCommandForUpdate(update, "reminder:toggle:bok_1:on"); got != "" {
		t.Fatalf("expected non-menu callback command to skip global throttling, got %q", got)
	}
}

func TestOperatorContextScopeForEvent(t *testing.T) {
	tests := []struct {
		name  string
		event botEngineOperatorEvent
		want  botEngineOperatorContextScope
	}{
		{
			name:  "dashboard loads dashboard and settings",
			event: botEngineOperatorEvent{Type: "message", Text: "/dashboard"},
			want:  botEngineOperatorContextScope{Dashboard: true, Settings: true},
		},
		{
			name:  "reminders only load reminders",
			event: botEngineOperatorEvent{Type: "callback", Data: "/reminders"},
			want:  botEngineOperatorContextScope{Reminders: true},
		},
		{
			name:  "conversation actions load conversations",
			event: botEngineOperatorEvent{Type: "callback", Data: "slots:dlg_1"},
			want:  botEngineOperatorContextScope{Conversations: true},
		},
		{
			name:  "faq loads faq scope",
			event: botEngineOperatorEvent{Type: "message", Text: "/faq"},
			want:  botEngineOperatorContextScope{FAQ: true},
		},
		{
			name:  "start keeps base scope only",
			event: botEngineOperatorEvent{Type: "message", Text: "/start"},
			want:  botEngineOperatorContextScope{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := operatorContextScopeForEvent(tt.event)
			if got != tt.want {
				t.Fatalf("unexpected scope: got %+v want %+v", got, tt.want)
			}
		})
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
