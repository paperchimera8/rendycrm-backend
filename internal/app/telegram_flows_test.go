package app

import (
	"context"
	"testing"

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

func TestNormalizeOperatorCommandReminderToggle(t *testing.T) {
	got := normalizeOperatorCommand("rmd:toggle:bok_123:off:2")
	if want := "reminder:toggle:bok_123:off"; got != want {
		t.Fatalf("unexpected reminder toggle normalization: got %q want %q", got, want)
	}
}

func TestTelegramButtonLabelReminderToggleUsesPosition(t *testing.T) {
	if got, want := telegramButtonLabel("rmd:toggle:bok_123:on:3"), "#3 Вкл"; got != want {
		t.Fatalf("unexpected reminder on label: got %q want %q", got, want)
	}
	if got, want := telegramButtonLabel("rmd:toggle:bok_123:off:4"), "#4 Выкл"; got != want {
		t.Fatalf("unexpected reminder off label: got %q want %q", got, want)
	}
}

func TestOperatorMainMenuButtonsIncludeReminders(t *testing.T) {
	buttons := operatorMainMenuButtons()
	for _, button := range buttons {
		if button == "/reminders" {
			return
		}
	}
	t.Fatalf("expected /reminders in operator main menu buttons, got %v", buttons)
}

func TestHandleOperatorCommandStartReturnsRemindersButton(t *testing.T) {
	server := &Server{}
	responses, err := server.handleOperatorCommand(context.Background(), OperatorBotBinding{
		WorkspaceID:     "ws_test",
		UserID:          "usr_test",
		TelegramChatID:  "123",
	}, "/start")
	if err != nil {
		t.Fatalf("handleOperatorCommand(/start): %v", err)
	}
	if len(responses) != 1 {
		t.Fatalf("expected single response, got %d", len(responses))
	}
	found := false
	for _, button := range responses[0].Buttons {
		if button == "/reminders" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /reminders button, got %v", responses[0].Buttons)
	}
}
