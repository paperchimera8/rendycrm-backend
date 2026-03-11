package app

import d "github.com/vital/rendycrm-app/internal/domain"

type Role = d.Role
type ConversationStatus = d.ConversationStatus
type MessageDirection = d.MessageDirection
type MessageSenderType = d.MessageSenderType
type MessageDeliveryStatus = d.MessageDeliveryStatus
type BookingStatus = d.BookingStatus
type ReviewStatus = d.ReviewStatus
type ChannelProvider = d.ChannelProvider
type ChannelKind = d.ChannelKind
type ChannelAccountScope = d.ChannelAccountScope
type EventType = d.EventType
type DailySlotStatus = d.DailySlotStatus
type AutomationIntent = d.AutomationIntent
type BotSessionScope = d.BotSessionScope
type BotSessionActorType = d.BotSessionActorType
type OutboundStatus = d.OutboundStatus
type OutboundKind = d.OutboundKind

type User = d.User
type Workspace = d.Workspace
type Session = d.Session
type Customer = d.Customer
type CustomerChannelIdentity = d.CustomerChannelIdentity
type ChannelAccount = d.ChannelAccount
type Message = d.Message
type Conversation = d.Conversation
type AvailabilityRule = d.AvailabilityRule
type AvailabilityException = d.AvailabilityException
type Slot = d.Slot
type SlotSettings = d.SlotSettings
type SlotColorPreset = d.SlotColorPreset
type SlotTemplate = d.SlotTemplate
type DailySlot = d.DailySlot
type WeekSlotDay = d.WeekSlotDay
type SlotHold = d.SlotHold
type Booking = d.Booking
type SlotEditorResponse = d.SlotEditorResponse
type Review = d.Review
type BotConfig = d.BotConfig
type FAQItem = d.FAQItem
type OperatorBotBinding = d.OperatorBotBinding
type OperatorBotLinkCode = d.OperatorBotLinkCode
type OperatorBotSettings = d.OperatorBotSettings
type MasterProfile = d.MasterProfile
type BotSession = d.BotSession
type InboundProfile = d.InboundProfile
type InboundMessageInput = d.InboundMessageInput
type InboxReceiveResult = d.InboxReceiveResult
type BotOutboundMessage = d.BotOutboundMessage
type OutboundMessage = d.OutboundMessage
type TelegramInlineButton = d.TelegramInlineButton
type TelegramOutboundPayload = d.TelegramOutboundPayload
type ClientBotRoute = d.ClientBotRoute
type AuditLog = d.AuditLog
type Dashboard = d.Dashboard
type AnalyticsOverview = d.AnalyticsOverview
type SSEEvent = d.SSEEvent

const (
	RoleAdmin    = d.RoleAdmin
	RoleOperator = d.RoleOperator

	ConversationNew    = d.ConversationNew
	ConversationAuto   = d.ConversationAuto
	ConversationHuman  = d.ConversationHuman
	ConversationBooked = d.ConversationBooked
	ConversationClosed = d.ConversationClosed

	ConversationOpen     = d.ConversationOpen
	ConversationResolved = d.ConversationResolved

	MessageInbound  = d.MessageInbound
	MessageOutbound = d.MessageOutbound

	MessageSenderCustomer = d.MessageSenderCustomer
	MessageSenderOperator = d.MessageSenderOperator
	MessageSenderBot      = d.MessageSenderBot

	MessageDeliveryQueued    = d.MessageDeliveryQueued
	MessageDeliverySent      = d.MessageDeliverySent
	MessageDeliveryDelivered = d.MessageDeliveryDelivered
	MessageDeliveryFailed    = d.MessageDeliveryFailed

	BookingPending   = d.BookingPending
	BookingConfirmed = d.BookingConfirmed
	BookingCompleted = d.BookingCompleted
	BookingCancelled = d.BookingCancelled

	ReviewOpen     = d.ReviewOpen
	ReviewResolved = d.ReviewResolved

	ChannelTelegram = d.ChannelTelegram
	ChannelWhatsApp = d.ChannelWhatsApp

	ChannelKindTelegramClient   = d.ChannelKindTelegramClient
	ChannelKindTelegramOperator = d.ChannelKindTelegramOperator
	ChannelKindWhatsAppTwilio   = d.ChannelKindWhatsAppTwilio

	ChannelAccountScopeWorkspace = d.ChannelAccountScopeWorkspace
	ChannelAccountScopeGlobal    = d.ChannelAccountScopeGlobal

	DailySlotFree    = d.DailySlotFree
	DailySlotHeld    = d.DailySlotHeld
	DailySlotBooked  = d.DailySlotBooked
	DailySlotBlocked = d.DailySlotBlocked

	IntentBookingRequest       = d.IntentBookingRequest
	IntentAvailabilityQuestion = d.IntentAvailabilityQuestion
	IntentPriceQuestion        = d.IntentPriceQuestion
	IntentReschedule           = d.IntentReschedule
	IntentCancel               = d.IntentCancel
	IntentFAQ                  = d.IntentFAQ
	IntentHumanRequest         = d.IntentHumanRequest
	IntentOther                = d.IntentOther

	BotSessionScopeOperator = d.BotSessionScopeOperator
	BotSessionScopeClient   = d.BotSessionScopeClient

	BotSessionActorUser     = d.BotSessionActorUser
	BotSessionActorCustomer = d.BotSessionActorCustomer

	OutboundStatusQueued     = d.OutboundStatusQueued
	OutboundStatusProcessing = d.OutboundStatusProcessing
	OutboundStatusSent       = d.OutboundStatusSent
	OutboundStatusDelivered  = d.OutboundStatusDelivered
	OutboundStatusFailed     = d.OutboundStatusFailed

	OutboundKindTelegramSendText   = d.OutboundKindTelegramSendText
	OutboundKindTelegramSendInline = d.OutboundKindTelegramSendInline
	OutboundKindTelegramEditInline = d.OutboundKindTelegramEditInline
	OutboundKindTelegramAnswerCBQ  = d.OutboundKindTelegramAnswerCBQ

	EventMessageNew         = d.EventMessageNew
	EventConversationAssign = d.EventConversationAssign
	EventBookingUpdated     = d.EventBookingUpdated
	EventReviewNew          = d.EventReviewNew
	EventDashboardUpdated   = d.EventDashboardUpdated
)
