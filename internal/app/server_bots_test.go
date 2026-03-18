package app

import "testing"

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
