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
