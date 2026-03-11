package telegram

import (
	"encoding/json"
	"errors"
	"strings"
)

func ParseUpdate(body []byte) (Update, error) {
	var update Update
	if err := json.Unmarshal(body, &update); err != nil {
		return Update{}, err
	}
	if update.Message == nil && update.CallbackQuery == nil {
		return Update{}, errors.New("unsupported telegram update")
	}
	return update, nil
}

func CommandText(update Update) (string, bool) {
	if update.Message == nil {
		return "", false
	}
	text := strings.TrimSpace(update.Message.Text)
	if !strings.HasPrefix(text, "/") {
		return "", false
	}
	return text, true
}

func CallbackData(update Update) (string, bool) {
	if update.CallbackQuery == nil {
		return "", false
	}
	data := strings.TrimSpace(update.CallbackQuery.Data)
	if data == "" {
		return "", false
	}
	return data, true
}
