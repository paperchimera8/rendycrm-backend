package domain

import "time"

type Role string

type ConversationStatus string

type MessageDirection string

type MessageSenderType string

type MessageDeliveryStatus string

type BookingStatus string

type ReviewStatus string

type ChannelProvider string
type ChannelKind string
type ChannelAccountScope string

type EventType string

type DailySlotStatus string

type AutomationIntent string

type BotSessionScope string

type BotSessionActorType string
type OutboundStatus string
type OutboundKind string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"

	ConversationNew    ConversationStatus = "new"
	ConversationAuto   ConversationStatus = "auto"
	ConversationHuman  ConversationStatus = "human"
	ConversationBooked ConversationStatus = "booked"
	ConversationClosed ConversationStatus = "closed"

	ConversationOpen     ConversationStatus = ConversationHuman
	ConversationResolved ConversationStatus = ConversationClosed

	MessageInbound  MessageDirection = "inbound"
	MessageOutbound MessageDirection = "outbound"

	MessageSenderCustomer MessageSenderType = "customer"
	MessageSenderOperator MessageSenderType = "operator"
	MessageSenderBot      MessageSenderType = "bot"

	MessageDeliveryQueued    MessageDeliveryStatus = "queued"
	MessageDeliverySent      MessageDeliveryStatus = "sent"
	MessageDeliveryDelivered MessageDeliveryStatus = "delivered"
	MessageDeliveryFailed    MessageDeliveryStatus = "failed"

	BookingPending   BookingStatus = "pending"
	BookingConfirmed BookingStatus = "confirmed"
	BookingCompleted BookingStatus = "completed"
	BookingCancelled BookingStatus = "cancelled"

	ReviewOpen     ReviewStatus = "open"
	ReviewResolved ReviewStatus = "resolved"

	ChannelTelegram ChannelProvider = "telegram"
	ChannelWhatsApp ChannelProvider = "whatsapp"

	ChannelKindTelegramClient   ChannelKind = "telegram_client"
	ChannelKindTelegramOperator ChannelKind = "telegram_operator"
	ChannelKindWhatsAppTwilio   ChannelKind = "whatsapp_twilio"

	ChannelAccountScopeWorkspace ChannelAccountScope = "workspace"
	ChannelAccountScopeGlobal    ChannelAccountScope = "global"

	DailySlotFree    DailySlotStatus = "free"
	DailySlotHeld    DailySlotStatus = "held"
	DailySlotBooked  DailySlotStatus = "booked"
	DailySlotBlocked DailySlotStatus = "blocked"

	IntentBookingRequest       AutomationIntent = "booking_request"
	IntentAvailabilityQuestion AutomationIntent = "availability_question"
	IntentPriceQuestion        AutomationIntent = "price_question"
	IntentReschedule           AutomationIntent = "reschedule"
	IntentCancel               AutomationIntent = "cancel"
	IntentFAQ                  AutomationIntent = "faq"
	IntentHumanRequest         AutomationIntent = "human_request"
	IntentOther                AutomationIntent = "other"

	BotSessionScopeOperator BotSessionScope = "operator"
	BotSessionScopeClient   BotSessionScope = "client"

	BotSessionActorUser     BotSessionActorType = "user"
	BotSessionActorCustomer BotSessionActorType = "customer"

	OutboundStatusQueued     OutboundStatus = "queued"
	OutboundStatusProcessing OutboundStatus = "processing"
	OutboundStatusSent       OutboundStatus = "sent"
	OutboundStatusDelivered  OutboundStatus = "delivered"
	OutboundStatusFailed     OutboundStatus = "failed"

	OutboundKindTelegramSendText   OutboundKind = "telegram.send_text"
	OutboundKindTelegramSendInline OutboundKind = "telegram.send_inline"
	OutboundKindTelegramEditInline OutboundKind = "telegram.edit_inline"
	OutboundKindTelegramAnswerCBQ  OutboundKind = "telegram.answer_callback"

	EventMessageNew         EventType = "message.new"
	EventConversationAssign EventType = "conversation.assigned"
	EventBookingUpdated     EventType = "booking.updated"
	EventReviewNew          EventType = "review.new"
	EventDashboardUpdated   EventType = "dashboard.updated"
)

type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"-"`
	Role     Role   `json:"role"`
}

type Workspace struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Timezone              string `json:"timezone,omitempty"`
	MasterPhoneRaw        string `json:"masterPhoneRaw,omitempty"`
	MasterPhoneNormalized string `json:"masterPhoneNormalized,omitempty"`
}

type Session struct {
	Token       string    `json:"token"`
	UserID      string    `json:"userId"`
	WorkspaceID string    `json:"workspaceId"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type Customer struct {
	ID               string                    `json:"id"`
	WorkspaceID      string                    `json:"workspaceId"`
	Name             string                    `json:"name"`
	Phone            string                    `json:"phone"`
	Email            string                    `json:"email"`
	Notes            string                    `json:"notes"`
	LastVisitAt      time.Time                 `json:"lastVisitAt"`
	BookingCount     int                       `json:"bookingCount"`
	PreferredChannel string                    `json:"preferredChannel"`
	Channels         []CustomerChannelIdentity `json:"channels,omitempty"`
}

type CustomerChannelIdentity struct {
	ID          string          `json:"id"`
	Provider    ChannelProvider `json:"provider"`
	ExternalID  string          `json:"externalId"`
	Username    string          `json:"username"`
	DisplayName string          `json:"displayName"`
}

type ChannelAccount struct {
	ID              string              `json:"id"`
	WorkspaceID     string              `json:"workspaceId"`
	Provider        ChannelProvider     `json:"provider"`
	Channel         ChannelProvider     `json:"channel"`
	ChannelKind     ChannelKind         `json:"channelKind"`
	AccountScope    ChannelAccountScope `json:"accountScope"`
	Name            string              `json:"name"`
	Connected       bool                `json:"connected"`
	IsEnabled       bool                `json:"isEnabled"`
	WebhookURL      string              `json:"webhookUrl"`
	AccountID       string              `json:"accountId"`
	BotUsername     string              `json:"botUsername"`
	TokenConfigured bool                `json:"tokenConfigured"`
	WebhookSecret   string              `json:"-"`
	EncryptedToken  string              `json:"-"`
}

type Message struct {
	ID             string            `json:"id"`
	ConversationID string            `json:"conversationId"`
	Direction      MessageDirection  `json:"direction"`
	SenderType     MessageSenderType `json:"senderType"`
	Text           string            `json:"text"`
	Status         string            `json:"status"`
	DeliveryStatus string            `json:"deliveryStatus"`
	ExternalID     string            `json:"externalId"`
	CreatedAt      time.Time         `json:"createdAt"`
}

type Conversation struct {
	ID                 string             `json:"id"`
	WorkspaceID        string             `json:"workspaceId"`
	CustomerID         string             `json:"customerId"`
	Provider           ChannelProvider    `json:"provider"`
	Channel            ChannelProvider    `json:"channel"`
	ExternalChatID     string             `json:"externalChatId"`
	Title              string             `json:"title"`
	Status             ConversationStatus `json:"status"`
	AssignedUserID     string             `json:"assignedUserId"`
	AssignedOperatorID string             `json:"assignedOperatorId"`
	UnreadCount        int                `json:"unreadCount"`
	AISummary          string             `json:"aiSummary"`
	Intent             AutomationIntent   `json:"intent"`
	LastMessageText    string             `json:"lastMessageText"`
	LastInboundAt      time.Time          `json:"lastInboundAt"`
	LastOutboundAt     time.Time          `json:"lastOutboundAt"`
	UpdatedAt          time.Time          `json:"updatedAt"`
}

type AvailabilityRule struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	DayOfWeek   int    `json:"dayOfWeek"`
	StartMinute int    `json:"startMinute"`
	EndMinute   int    `json:"endMinute"`
	Enabled     bool   `json:"enabled"`
}

type AvailabilityException struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	StartsAt    time.Time `json:"startsAt"`
	EndsAt      time.Time `json:"endsAt"`
	Reason      string    `json:"reason"`
}

type Slot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type SlotSettings struct {
	WorkspaceID            string `json:"workspaceId"`
	Timezone               string `json:"timezone"`
	DefaultDurationMinutes int    `json:"defaultDurationMinutes"`
	GenerationHorizonDays  int    `json:"generationHorizonDays"`
}

type SlotColorPreset struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
	Hex         string `json:"hex"`
	Position    int    `json:"position"`
}

type SlotTemplate struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspaceId"`
	Weekday         int    `json:"weekday"`
	StartMinute     int    `json:"startMinute"`
	DurationMinutes int    `json:"durationMinutes"`
	ColorPresetID   string `json:"colorPresetId"`
	ColorName       string `json:"colorName"`
	ColorHex        string `json:"colorHex"`
	Position        int    `json:"position"`
	Enabled         bool   `json:"enabled"`
}

type DailySlot struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspaceId"`
	SlotDate         string          `json:"slotDate"`
	StartsAt         time.Time       `json:"startsAt"`
	EndsAt           time.Time       `json:"endsAt"`
	DurationMinutes  int             `json:"durationMinutes"`
	ColorPresetID    string          `json:"colorPresetId"`
	ColorName        string          `json:"colorName"`
	ColorHex         string          `json:"colorHex"`
	Position         int             `json:"position"`
	Status           DailySlotStatus `json:"status"`
	SourceTemplateID string          `json:"sourceTemplateId"`
	IsManual         bool            `json:"isManual"`
	Note             string          `json:"note"`
	BookingID        string          `json:"bookingId"`
	CustomerName     string          `json:"customerName"`
}

type WeekSlotDay struct {
	Date    string      `json:"date"`
	Weekday int         `json:"weekday"`
	Label   string      `json:"label"`
	Slots   []DailySlot `json:"slots"`
}

type SlotHold struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	CustomerID  string    `json:"customerId"`
	DailySlotID string    `json:"dailySlotId"`
	StartsAt    time.Time `json:"startsAt"`
	EndsAt      time.Time `json:"endsAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type Booking struct {
	ID           string        `json:"id"`
	WorkspaceID  string        `json:"workspaceId"`
	CustomerID   string        `json:"customerId"`
	CustomerName string        `json:"customerName"`
	DailySlotID  string        `json:"dailySlotId"`
	StartsAt     time.Time     `json:"startsAt"`
	EndsAt       time.Time     `json:"endsAt"`
	Amount       int           `json:"amount"`
	Status       BookingStatus `json:"status"`
	Source       string        `json:"source"`
	Notes        string        `json:"notes"`
}

type SlotEditorResponse struct {
	Settings      SlotSettings      `json:"settings"`
	Colors        []SlotColorPreset `json:"colors"`
	WeekTemplates []SlotTemplate    `json:"weekTemplates"`
	DaySlots      []DailySlot       `json:"daySlots"`
}

type Review struct {
	ID          string       `json:"id"`
	WorkspaceID string       `json:"workspaceId"`
	CustomerID  string       `json:"customerId"`
	BookingID   string       `json:"bookingId"`
	Rating      int          `json:"rating"`
	Text        string       `json:"text"`
	Status      ReviewStatus `json:"status"`
	CreatedAt   time.Time    `json:"createdAt"`
}

type BotConfig struct {
	WorkspaceID    string `json:"workspaceId"`
	AutoReply      bool   `json:"autoReply"`
	HandoffEnabled bool   `json:"handoffEnabled"`
	FAQCount       int    `json:"faqCount"`
	Tone           string `json:"tone"`
	WelcomeMessage string `json:"welcomeMessage"`
	HandoffMessage string `json:"handoffMessage"`
}

type FAQItem struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Question    string `json:"question"`
	Answer      string `json:"answer"`
}

type OperatorBotBinding struct {
	UserID         string    `json:"userId"`
	WorkspaceID    string    `json:"workspaceId"`
	TelegramUserID string    `json:"telegramUserId"`
	TelegramChatID string    `json:"telegramChatId"`
	LinkedAt       time.Time `json:"linkedAt"`
	IsActive       bool      `json:"isActive"`
}

type OperatorBotLinkCode struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	WorkspaceID string    `json:"workspaceId"`
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expiresAt"`
	DeepLink    string    `json:"deepLink"`
}

type OperatorBotSettings struct {
	Binding            *OperatorBotBinding  `json:"binding"`
	PendingLink        *OperatorBotLinkCode `json:"pendingLink"`
	BotUsername        string               `json:"botUsername"`
	OperatorWebhookURL string               `json:"operatorWebhookUrl"`
	TokenConfigured    bool                 `json:"tokenConfigured"`
}

type MasterProfile struct {
	WorkspaceID           string `json:"workspaceId"`
	MasterPhoneRaw        string `json:"masterPhoneRaw"`
	MasterPhoneNormalized string `json:"masterPhoneNormalized"`
	TelegramEnabled       bool   `json:"telegramEnabled"`
}

type BotSession struct {
	ID          string              `json:"id"`
	WorkspaceID string              `json:"workspaceId"`
	BotKind     ChannelKind         `json:"botKind"`
	Scope       BotSessionScope     `json:"scope"`
	ActorType   BotSessionActorType `json:"actorType"`
	ActorID     string              `json:"actorId"`
	State       string              `json:"state"`
	Payload     string              `json:"payload"`
	ExpiresAt   time.Time           `json:"expiresAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

type InboundProfile struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Phone    string `json:"phone"`
}

type InboundMessageInput struct {
	Provider          ChannelProvider `json:"provider"`
	ChannelAccountID  string          `json:"channelAccountId"`
	ExternalChatID    string          `json:"externalChatId"`
	ExternalMessageID string          `json:"externalMessageId"`
	Text              string          `json:"text"`
	Timestamp         time.Time       `json:"timestamp"`
	Profile           InboundProfile  `json:"profile"`
}

type InboxReceiveResult struct {
	Conversation Conversation         `json:"conversation"`
	Message      Message              `json:"message"`
	Customer     Customer             `json:"customer"`
	Responses    []BotOutboundMessage `json:"responses"`
	Stored       bool                 `json:"stored"`
}

type BotOutboundMessage struct {
	ChatID         string   `json:"chatId"`
	Text           string   `json:"text"`
	Buttons        []string `json:"buttons,omitempty"`
	Channel        string   `json:"channel,omitempty"`
	ConversationID string   `json:"conversationId,omitempty"`
}

type OutboundMessage struct {
	ID                string          `json:"id"`
	WorkspaceID       string          `json:"workspaceId"`
	Channel           ChannelProvider `json:"channel"`
	ChannelKind       ChannelKind     `json:"channelKind"`
	ChannelAccountID  string          `json:"channelAccountId"`
	ConversationID    string          `json:"conversationId"`
	MessageID         string          `json:"messageId"`
	Kind              OutboundKind    `json:"kind"`
	Payload           string          `json:"payload"`
	Status            OutboundStatus  `json:"status"`
	RetryCount        int             `json:"retryCount"`
	LastError         string          `json:"lastError"`
	NextAttemptAt     time.Time       `json:"nextAttemptAt"`
	ProviderMessageID string          `json:"providerMessageId"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type TelegramInlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callbackData"`
}

type TelegramOutboundPayload struct {
	ChatID       string                 `json:"chatId"`
	Text         string                 `json:"text"`
	MessageID    int64                  `json:"messageId,omitempty"`
	Buttons      []TelegramInlineButton `json:"buttons,omitempty"`
	ParseMode    string                 `json:"parseMode,omitempty"`
	CallbackID   string                 `json:"callbackId,omitempty"`
	CallbackText string                 `json:"callbackText,omitempty"`
	ShowAlert    bool                   `json:"showAlert,omitempty"`
}

type ClientBotRoute struct {
	ID                            string    `json:"id"`
	ChannelAccountID              string    `json:"channelAccountId"`
	ExternalChatID                string    `json:"externalChatId"`
	SelectedWorkspaceID           string    `json:"selectedWorkspaceId"`
	SelectedMasterPhoneNormalized string    `json:"selectedMasterPhoneNormalized"`
	State                         string    `json:"state"`
	ExpiresAt                     time.Time `json:"expiresAt"`
	UpdatedAt                     time.Time `json:"updatedAt"`
}

type AuditLog struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	UserID      string    `json:"userId"`
	Action      string    `json:"action"`
	EntityType  string    `json:"entityType"`
	EntityID    string    `json:"entityId"`
	Payload     string    `json:"payload"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Dashboard struct {
	TodayBookings        int `json:"todayBookings"`
	AwaitingConfirmation int `json:"awaitingConfirmation"`
	CancelledBookings    int `json:"cancelledBookings"`
	NewMessages          int `json:"newMessages"`
	NewReviews           int `json:"newReviews"`
	ActiveConversations  int `json:"activeConversations"`
}

type AnalyticsOverview struct {
	Revenue               int `json:"revenue"`
	ConfirmationRate      int `json:"confirmationRate"`
	NoShowRate            int `json:"noShowRate"`
	RepeatBookings        int `json:"repeatBookings"`
	ConversationToBooking int `json:"conversationToBooking"`
}

type SSEEvent struct {
	Type EventType `json:"type"`
	Data any       `json:"data"`
}
