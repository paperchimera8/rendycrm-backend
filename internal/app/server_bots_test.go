package app

import (
	"testing"
	"time"
)

func TestTelegramWebhookURL(t *testing.T) {
	server := &Server{cfg: Config{PublicBaseURL: "https://rendycrm.ru/api"}}

	clientURL, err := server.telegramWebhookURL(ChannelAccount{
		ID:            "cha_1",
		Provider:      ChannelTelegram,
		ChannelKind:   ChannelKindTelegramClient,
		WebhookSecret: "secret-1",
	})
	if err != nil {
		t.Fatalf("client webhook url: %v", err)
	}
	if clientURL != "https://rendycrm.ru/api/webhooks/telegram/client/cha_1/secret-1" {
		t.Fatalf("unexpected client webhook url: %s", clientURL)
	}

	operatorURL, err := server.telegramWebhookURL(ChannelAccount{
		Provider:    ChannelTelegram,
		ChannelKind: ChannelKindTelegramOperator,
	})
	if err != nil {
		t.Fatalf("operator webhook url: %v", err)
	}
	if operatorURL != "https://rendycrm.ru/api/webhooks/telegram/operator" {
		t.Fatalf("unexpected operator webhook url: %s", operatorURL)
	}
}

func TestTelegramStartPayload(t *testing.T) {
	payload, ok := telegramStartPayload("/start master_79991112233")
	if !ok {
		t.Fatal("expected /start payload to be detected")
	}
	if payload != "master_79991112233" {
		t.Fatalf("unexpected payload: %s", payload)
	}

	payload, ok = telegramStartPayload("/start@rendycrmbot")
	if !ok {
		t.Fatal("expected /start@bot to be detected")
	}
	if payload != "" {
		t.Fatalf("expected empty payload, got %s", payload)
	}

	if _, ok := telegramStartPayload("/dialogs"); ok {
		t.Fatal("did not expect non-start command to match")
	}
}

func TestTelegramMasterPhoneFromStartPayload(t *testing.T) {
	if got := telegramMasterPhoneFromStartPayload("master_79991112233"); got != "79991112233" {
		t.Fatalf("unexpected master payload parse: %s", got)
	}
	if got := telegramMasterPhoneFromStartPayload("phone_+7 999 111-22-33"); got != "+7 999 111-22-33" {
		t.Fatalf("unexpected phone payload parse: %s", got)
	}
	if got := telegramMasterPhoneFromStartPayload("79991112233"); got != "79991112233" {
		t.Fatalf("unexpected raw payload parse: %s", got)
	}
}

func TestTelegramClientWelcomeText(t *testing.T) {
	if got := telegramClientWelcomeText(); got == "" {
		t.Fatal("expected welcome text")
	}
	if got := telegramClientMasterPhonePromptText(); got == "" {
		t.Fatal("expected prompt text")
	}
	if telegramClientWelcomeText() == telegramClientMasterPhonePromptText() {
		t.Fatal("expected welcome and prompt texts to differ")
	}
}

func TestShouldSkipTelegramRoutePrompt(t *testing.T) {
	now := time.Now().UTC()
	if !shouldSkipTelegramRoutePrompt(ClientBotRoute{
		ChannelAccountID: "cha_1",
		State:            "awaiting_master_phone",
		UpdatedAt:        now.Add(-time.Minute),
		ExpiresAt:        now.Add(time.Hour),
	}, "awaiting_master_phone", "", "", now) {
		t.Fatal("expected awaiting prompt to be skipped inside cooldown")
	}

	if !shouldSkipTelegramRoutePrompt(ClientBotRoute{
		ChannelAccountID:              "cha_1",
		State:                         "ready",
		SelectedWorkspaceID:           "ws_1",
		SelectedMasterPhoneNormalized: "79991112233",
		UpdatedAt:                     now.Add(-time.Minute),
		ExpiresAt:                     now.Add(time.Hour),
	}, "ready", "ws_1", "79991112233", now) {
		t.Fatal("expected ready prompt to be skipped for same master inside cooldown")
	}

	if shouldSkipTelegramRoutePrompt(ClientBotRoute{
		ChannelAccountID:              "cha_1",
		State:                         "ready",
		SelectedWorkspaceID:           "ws_1",
		SelectedMasterPhoneNormalized: "79991112233",
		UpdatedAt:                     now.Add(-3 * time.Minute),
		ExpiresAt:                     now.Add(time.Hour),
	}, "ready", "ws_1", "79991112233", now) {
		t.Fatal("did not expect cooldown to suppress old prompt")
	}
}
