package app

import "testing"

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
