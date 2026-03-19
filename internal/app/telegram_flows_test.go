package app

import (
	"context"
	"testing"
	"time"

	tgapi "github.com/vital/rendycrm-app/internal/telegram"
)

func TestTelegramCallbackExternalMessageIDUsesSourceMessageAndAction(t *testing.T) {
	first := tgapi.Update{
		CallbackQuery: &tgapi.CallbackQuery{
			ID:   "cbq-first",
			Data: "client:slots",
			Message: &tgapi.Message{
				MessageID: 42,
				Chat:      tgapi.Chat{ID: 1001},
			},
		},
	}
	second := tgapi.Update{
		CallbackQuery: &tgapi.CallbackQuery{
			ID:   "cbq-second",
			Data: "client:slots",
			Message: &tgapi.Message{
				MessageID: 42,
				Chat:      tgapi.Chat{ID: 1001},
			},
		},
	}

	if got, want := telegramCallbackExternalMessageID(first), "cbqmsg:1001:42:client:slots"; got != want {
		t.Fatalf("unexpected callback dedup id: got %q want %q", got, want)
	}
	if telegramCallbackExternalMessageID(first) != telegramCallbackExternalMessageID(second) {
		t.Fatal("expected repeated taps on the same callback message to reuse the dedup id")
	}
}

func TestTelegramCallbackExternalMessageIDFallsBackToCallbackID(t *testing.T) {
	update := tgapi.Update{
		CallbackQuery: &tgapi.CallbackQuery{
			ID:   "cbq-fallback",
			Data: "client:prices",
		},
	}

	if got, want := telegramCallbackExternalMessageID(update), "cbq:cbq-fallback"; got != want {
		t.Fatalf("unexpected fallback callback dedup id: got %q want %q", got, want)
	}
}

func TestHandleTelegramClientCallbackIgnoresCallbackWithoutMessage(t *testing.T) {
	server := &Server{}
	update := tgapi.Update{
		CallbackQuery: &tgapi.CallbackQuery{
			ID:   "cbq-no-message",
			Data: "client:human",
		},
	}

	if err := server.handleTelegramClientCallback(context.Background(), ChannelAccount{}, update); err != nil {
		t.Fatalf("expected callback without message to be ignored safely, got %v", err)
	}
}

func TestTelegramCallbackActionKeySameCommandSameSourceIsStable(t *testing.T) {
	first := telegramCallbackActionKey("account-1", ChannelKindTelegramClient, "1001", 42, "client:slots")
	second := telegramCallbackActionKey("account-1", ChannelKindTelegramClient, "1001", 42, " client:slots ")

	if first != second {
		t.Fatalf("expected callback action key to ignore surrounding whitespace, got %q and %q", first, second)
	}
}

func TestTelegramCallbackActionKeyDifferentCommandDiffers(t *testing.T) {
	first := telegramCallbackActionKey("account-1", ChannelKindTelegramClient, "1001", 42, "client:slots")
	second := telegramCallbackActionKey("account-1", ChannelKindTelegramClient, "1001", 42, "client:prices")

	if first == second {
		t.Fatalf("expected different commands to produce different callback action keys, got %q", first)
	}
}

func TestTelegramInboundDeliveryKeyUsesCallbackIDWhenPresent(t *testing.T) {
	first := telegramInboundDeliveryKey("account-1", ChannelKindTelegramOperator, "1001", 42, "cbq-1")
	second := telegramInboundDeliveryKey("account-1", ChannelKindTelegramOperator, "1001", 42, "cbq-1")
	third := telegramInboundDeliveryKey("account-1", ChannelKindTelegramOperator, "1001", 42, "cbq-2")

	if first != second {
		t.Fatalf("expected identical callback ids to reuse the same inbound key, got %q and %q", first, second)
	}
	if first == third {
		t.Fatalf("expected different callback ids to produce different inbound keys, got %q", first)
	}
}

func TestHandleTelegramOperatorUpdateIgnoresCallbackWithoutMessage(t *testing.T) {
	server := &Server{}
	update := tgapi.Update{
		UpdateID: 77,
		CallbackQuery: &tgapi.CallbackQuery{
			ID:   "cbq-no-message",
			Data: "/dialogs",
			From: tgapi.User{ID: 123},
		},
	}

	if err := server.handleTelegramOperatorUpdate(context.Background(), ChannelAccount{}, update); err != nil {
		t.Fatalf("expected operator callback without message to be ignored safely, got %v", err)
	}
}

func TestIsTelegramMasterPhonePromptRequest(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "button label infinitive", text: "Ввести номер мастера", want: true},
		{name: "button label imperative", text: "Введите номер мастера", want: true},
		{name: "full prompt text", text: "Введите номер мастера, к которому хотите записаться.", want: true},
		{name: "plain phone", text: "+7 999 111-22-33", want: false},
		{name: "arbitrary text", text: "привет", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTelegramMasterPhonePromptRequest(tt.text); got != tt.want {
				t.Fatalf("unexpected prompt request detection for %q: got %t want %t", tt.text, got, tt.want)
			}
		})
	}
}

func TestShouldClearClientRoute(t *testing.T) {
	now := time.Date(2026, time.March, 19, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		route ClientBotRoute
		want  bool
	}{
		{
			name: "awaiting master phone is preserved",
			route: ClientBotRoute{
				ChannelAccountID: "cha_1",
				ExternalChatID:   "chat_1",
				State:            "awaiting_master_phone",
				ExpiresAt:        now.Add(time.Hour),
			},
			want: false,
		},
		{
			name: "expired route is cleared",
			route: ClientBotRoute{
				ChannelAccountID:    "cha_1",
				ExternalChatID:      "chat_1",
				State:               "ready",
				SelectedWorkspaceID: "ws_1",
				ExpiresAt:           now.Add(-time.Minute),
			},
			want: true,
		},
		{
			name: "ready route without workspace is cleared",
			route: ClientBotRoute{
				ChannelAccountID: "cha_1",
				ExternalChatID:   "chat_1",
				State:            "ready",
				ExpiresAt:        now.Add(time.Hour),
			},
			want: true,
		},
		{
			name: "ready route with workspace is kept",
			route: ClientBotRoute{
				ChannelAccountID:    "cha_1",
				ExternalChatID:      "chat_1",
				State:               "ready",
				SelectedWorkspaceID: "ws_1",
				ExpiresAt:           now.Add(time.Hour),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldClearClientRoute(tt.route, now); got != tt.want {
				t.Fatalf("unexpected clear decision: got %t want %t", got, tt.want)
			}
		})
	}
}
