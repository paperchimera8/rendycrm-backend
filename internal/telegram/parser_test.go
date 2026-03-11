package telegram

import "testing"

func TestParseUpdateMessageAndCommand(t *testing.T) {
	update, err := ParseUpdate([]byte(`{
		"update_id": 11,
		"message": {
			"message_id": 22,
			"date": 1710000000,
			"text": "/start hello",
			"chat": {"id": 33},
			"from": {"id": 44, "username": "user", "first_name": "Test"}
		}
	}`))
	if err != nil {
		t.Fatalf("parse update: %v", err)
	}
	if update.Message == nil || update.Message.Chat.ID != 33 {
		t.Fatalf("unexpected update: %+v", update)
	}
	command, ok := CommandText(update)
	if !ok || command != "/start hello" {
		t.Fatalf("unexpected command parse: %q %t", command, ok)
	}
}

func TestParseUpdateCallback(t *testing.T) {
	update, err := ParseUpdate([]byte(`{
		"update_id": 51,
		"callback_query": {
			"id": "cbq-1",
			"data": "dlg:open:123",
			"from": {"id": 44, "username": "user", "first_name": "Test"},
			"message": {"message_id": 22, "date": 1710000000, "text": "x", "chat": {"id": 33}}
		}
	}`))
	if err != nil {
		t.Fatalf("parse update: %v", err)
	}
	data, ok := CallbackData(update)
	if !ok || data != "dlg:open:123" {
		t.Fatalf("unexpected callback parse: %q %t", data, ok)
	}
}
