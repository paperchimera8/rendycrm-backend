package telegram

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	EditedMessage *Message       `json:"edited_message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int64    `json:"message_id"`
	Date      int64    `json:"date"`
	Text      string   `json:"text"`
	Chat      Chat     `json:"chat"`
	From      *User    `json:"from,omitempty"`
	Contact   *Contact `json:"contact,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	UserID      int64  `json:"user_id"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type ReplyKeyboardMarkup struct {
	Keyboard        [][]ReplyKeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool                    `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool                    `json:"one_time_keyboard,omitempty"`
}

type ReplyKeyboardButton struct {
	Text string `json:"text"`
}

type SendMessageRequest struct {
	ChatID      string `json:"chat_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

type EditMessageTextRequest struct {
	ChatID      string `json:"chat_id"`
	MessageID   int64  `json:"message_id"`
	Text        string `json:"text"`
	ParseMode   string `json:"parse_mode,omitempty"`
	ReplyMarkup any    `json:"reply_markup,omitempty"`
}

type AnswerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

type APIResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Parameters  struct {
		RetryAfter int `json:"retry_after,omitempty"`
	} `json:"parameters,omitempty"`
	Result T `json:"result"`
}

type MessageResponse struct {
	MessageID int64 `json:"message_id"`
}
