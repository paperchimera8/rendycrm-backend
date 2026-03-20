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
