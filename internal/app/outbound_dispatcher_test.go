package app

import (
	"errors"
	"testing"

	tgapi "github.com/vital/rendycrm-app/internal/telegram"
)

func TestIsTelegramEditNoopError(t *testing.T) {
	err := &tgapi.APIError{
		Method:      "editMessageText",
		StatusCode:  400,
		Description: "Bad Request: message is not modified",
	}

	if !isTelegramEditNoopError(err) {
		t.Fatal("expected telegram edit noop error to be detected")
	}
}

func TestIsTelegramEditNoopErrorRejectsOtherErrors(t *testing.T) {
	tests := []error{
		errors.New("boom"),
		&tgapi.APIError{
			Method:      "editMessageText",
			StatusCode:  400,
			Description: "Bad Request: chat not found",
		},
		&tgapi.APIError{
			Method:      "sendMessage",
			StatusCode:  429,
			Description: "Too Many Requests",
		},
	}

	for _, err := range tests {
		if isTelegramEditNoopError(err) {
			t.Fatalf("did not expect error %v to be treated as edit noop", err)
		}
	}
}

func TestShouldFallbackTelegramEditToSend(t *testing.T) {
	err := &tgapi.APIError{
		Method:      "editMessageText",
		StatusCode:  400,
		Description: "Bad Request: message to edit not found",
	}

	if !shouldFallbackTelegramEditToSend(err) {
		t.Fatal("expected 400 edit error to fallback to send")
	}
}

func TestShouldFallbackTelegramEditToSendRejectsNon400(t *testing.T) {
	tests := []error{
		errors.New("boom"),
		&tgapi.APIError{
			Method:      "editMessageText",
			StatusCode:  429,
			Description: "Too Many Requests",
		},
	}

	for _, err := range tests {
		if shouldFallbackTelegramEditToSend(err) {
			t.Fatalf("did not expect error %v to fallback to send", err)
		}
	}
}

func TestShouldConfirmTelegramEdit(t *testing.T) {
	err := &tgapi.APIError{
		Method:      "editMessageText",
		StatusCode:  400,
		Description: "Bad Request",
	}

	if !shouldConfirmTelegramEdit(err) {
		t.Fatal("expected 400 edit error to be confirmed with a second edit attempt")
	}
}

func TestShouldConfirmTelegramEditRejectsNon400(t *testing.T) {
	tests := []error{
		errors.New("boom"),
		&tgapi.APIError{
			Method:      "editMessageText",
			StatusCode:  429,
			Description: "Too Many Requests",
		},
	}

	for _, err := range tests {
		if shouldConfirmTelegramEdit(err) {
			t.Fatalf("did not expect error %v to trigger edit confirmation", err)
		}
	}
}

func TestShouldAssumeTelegramEditAppliedForOperatorMenu(t *testing.T) {
	item := OutboundMessage{ChannelKind: ChannelKindTelegramOperator}
	payload := TelegramOutboundPayload{
		ChatID:    "1348661149",
		MessageID: 193,
		Buttons: []TelegramInlineButton{
			{Text: "FAQ", CallbackData: "/faq"},
		},
	}
	err := &tgapi.APIError{
		Method:      "editMessageText",
		StatusCode:  400,
		Description: "Bad Request",
	}

	if !shouldAssumeTelegramEditApplied(item, payload, err) {
		t.Fatal("expected operator menu 400 to be treated as applied")
	}
}

func TestShouldAssumeTelegramEditAppliedRejectsClientOrButtonlessEdits(t *testing.T) {
	tests := []struct {
		name    string
		item    OutboundMessage
		payload TelegramOutboundPayload
		err     error
	}{
		{
			name: "client",
			item: OutboundMessage{ChannelKind: ChannelKindTelegramClient},
			payload: TelegramOutboundPayload{
				ChatID:    "1348661149",
				MessageID: 193,
				Buttons:   []TelegramInlineButton{{Text: "FAQ", CallbackData: "/faq"}},
			},
			err: &tgapi.APIError{Method: "editMessageText", StatusCode: 400, Description: "Bad Request"},
		},
		{
			name: "no buttons",
			item: OutboundMessage{ChannelKind: ChannelKindTelegramOperator},
			payload: TelegramOutboundPayload{
				ChatID:    "1348661149",
				MessageID: 193,
			},
			err: &tgapi.APIError{Method: "editMessageText", StatusCode: 400, Description: "Bad Request"},
		},
	}

	for _, test := range tests {
		if shouldAssumeTelegramEditApplied(test.item, test.payload, test.err) {
			t.Fatalf("did not expect %s case to be treated as applied", test.name)
		}
	}
}
