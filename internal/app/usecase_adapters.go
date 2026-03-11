package app

import (
	"context"
	"time"

	"github.com/vital/rendycrm-app/internal/usecase"
)

type ApplicationServices struct {
	Dialogs      usecase.DialogService
	Bookings     usecase.BookingService
	Customers    usecase.CustomerService
	Reviews      usecase.ReviewService
	Channels     usecase.ChannelService
	BotSettings  usecase.BotSettingsService
	BotSessions  usecase.BotSessionService
	Inbox        usecase.InboxService
	OperatorLink usecase.OperatorLinkService
}

func newApplicationServices(repo *Repository) ApplicationServices {
	policy := usecase.DefaultPolicy{}
	adapter := usecaseRepositoryAdapter{repo: repo}
	return ApplicationServices{
		Dialogs:      usecase.NewDialogService(adapter, policy),
		Bookings:     usecase.NewBookingService(adapter, policy),
		Customers:    usecase.NewCustomerService(adapter, policy),
		Reviews:      usecase.NewReviewService(adapter, policy),
		Channels:     usecase.NewChannelService(adapter, policy),
		BotSettings:  usecase.NewBotSettingsService(adapter, policy),
		BotSessions:  usecase.NewBotSessionService(adapter, policy),
		Inbox:        usecase.NewInboxService(adapter),
		OperatorLink: usecase.NewOperatorLinkService(adapter, policy),
	}
}

type usecaseRepositoryAdapter struct {
	repo *Repository
}

func (a usecaseRepositoryAdapter) ReplyToDialog(ctx context.Context, workspaceID, conversationID, userID, text string) (usecase.DialogReplyResult, error) {
	_, message, err := a.repo.Reply(ctx, workspaceID, conversationID, userID, text)
	if err != nil {
		return usecase.DialogReplyResult{}, err
	}
	return usecase.DialogReplyResult{ConversationID: conversationID, MessageID: message.ID}, nil
}

func (a usecaseRepositoryAdapter) AssignDialog(ctx context.Context, workspaceID, conversationID, userID string) error {
	_, err := a.repo.AssignConversation(ctx, workspaceID, conversationID, userID)
	return err
}

func (a usecaseRepositoryAdapter) SetDialogAutomation(ctx context.Context, workspaceID, conversationID, status, intent string) error {
	_, err := a.repo.UpdateConversationAutomation(ctx, workspaceID, conversationID, ConversationStatus(status), AutomationIntent(intent))
	return err
}

func (a usecaseRepositoryAdapter) SetDialogStatus(ctx context.Context, workspaceID, conversationID, status string) error {
	_, err := a.repo.UpdateConversationStatus(ctx, workspaceID, conversationID, ConversationStatus(status))
	return err
}

func (a usecaseRepositoryAdapter) UpdateCustomerName(ctx context.Context, workspaceID, customerID, name string) (usecase.CustomerResult, error) {
	customer, err := a.repo.UpdateCustomerName(ctx, workspaceID, customerID, name)
	if err != nil {
		return usecase.CustomerResult{}, err
	}
	return usecase.CustomerResult{ID: customer.ID, Name: customer.Name}, nil
}

func (a usecaseRepositoryAdapter) UpdateReviewStatus(ctx context.Context, workspaceID, reviewID, status string) (usecase.ReviewResult, error) {
	review, err := a.repo.UpdateReviewStatus(ctx, workspaceID, reviewID, ReviewStatus(status))
	if err != nil {
		return usecase.ReviewResult{}, err
	}
	return usecase.ReviewResult{ID: review.ID, Status: string(review.Status)}, nil
}

func (a usecaseRepositoryAdapter) AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	return a.repo.CreateAuditLog(ctx, workspaceID, userID, action, entityType, entityID, payload)
}

func mapBookingResult(booking Booking) usecase.BookingResult {
	return usecase.BookingResult{
		ID:        booking.ID,
		StartsAt:  booking.StartsAt,
		EndsAt:    booking.EndsAt,
		Status:    string(booking.Status),
		DailySlot: booking.DailySlotID,
	}
}

func (a usecaseRepositoryAdapter) CreatePendingBookingForSlot(ctx context.Context, workspaceID, customerID, dailySlotID, notes string) (usecase.BookingResult, error) {
	booking, _, err := a.repo.CreateBookingForDailySlot(ctx, workspaceID, customerID, dailySlotID, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) CreatePendingBookingForRange(ctx context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, notes string) (usecase.BookingResult, error) {
	booking, _, err := a.repo.CreateBooking(ctx, workspaceID, customerID, startsAt, endsAt, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) CreateConfirmedBookingForSlot(ctx context.Context, workspaceID, customerID, dailySlotID string, amount int, notes string) (usecase.BookingResult, error) {
	booking, err := a.repo.CreateConfirmedBookingForDailySlot(ctx, workspaceID, customerID, dailySlotID, amount, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) CreateConfirmedBookingForRange(ctx context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, amount int, notes string) (usecase.BookingResult, error) {
	booking, err := a.repo.CreateConfirmedBooking(ctx, workspaceID, customerID, startsAt, endsAt, amount, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) ConfirmBooking(ctx context.Context, workspaceID, bookingID string, amount int) (usecase.BookingResult, error) {
	booking, err := a.repo.UpdateBookingStatus(ctx, workspaceID, bookingID, BookingConfirmed, &amount)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) CancelBooking(ctx context.Context, workspaceID, bookingID string) (usecase.BookingResult, error) {
	booking, err := a.repo.UpdateBookingStatus(ctx, workspaceID, bookingID, BookingCancelled, nil)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) CompleteBooking(ctx context.Context, workspaceID, bookingID string, amount int) (usecase.BookingResult, error) {
	booking, err := a.repo.UpdateBookingStatus(ctx, workspaceID, bookingID, BookingCompleted, &amount)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) ReschedulePendingToSlot(ctx context.Context, workspaceID, bookingID, dailySlotID, notes string) (usecase.BookingResult, error) {
	booking, err := a.repo.RescheduleBookingToDailySlot(ctx, workspaceID, bookingID, dailySlotID, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) ReschedulePendingToRange(ctx context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, notes string) (usecase.BookingResult, error) {
	booking, err := a.repo.RescheduleBooking(ctx, workspaceID, bookingID, startsAt, endsAt, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) RescheduleConfirmedToSlot(ctx context.Context, workspaceID, bookingID, dailySlotID string, amount int, notes string) (usecase.BookingResult, error) {
	booking, err := a.repo.RescheduleConfirmedBookingToDailySlot(ctx, workspaceID, bookingID, dailySlotID, amount, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) RescheduleConfirmedToRange(ctx context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, amount int, notes string) (usecase.BookingResult, error) {
	booking, err := a.repo.RescheduleConfirmedBooking(ctx, workspaceID, bookingID, startsAt, endsAt, amount, notes)
	return mapBookingResult(booking), err
}

func (a usecaseRepositoryAdapter) LoadBotConfig(ctx context.Context, workspaceID string) (usecase.BotConfigState, []usecase.FAQItem, error) {
	config, faqItems, err := a.repo.BotConfig(ctx, workspaceID)
	if err != nil {
		return usecase.BotConfigState{}, nil, err
	}
	result := usecase.BotConfigState{
		AutoReply:      config.AutoReply,
		HandoffEnabled: config.HandoffEnabled,
		Tone:           config.Tone,
		WelcomeMessage: config.WelcomeMessage,
		HandoffMessage: config.HandoffMessage,
	}
	items := make([]usecase.FAQItem, 0, len(faqItems))
	for _, item := range faqItems {
		items = append(items, usecase.FAQItem{ID: item.ID, Question: item.Question, Answer: item.Answer})
	}
	return result, items, nil
}

func (a usecaseRepositoryAdapter) SaveBotConfig(ctx context.Context, workspaceID string, config usecase.BotConfigState, faq []usecase.FAQItem) error {
	items := make([]FAQItem, 0, len(faq))
	for _, item := range faq {
		items = append(items, FAQItem{ID: item.ID, Question: item.Question, Answer: item.Answer, WorkspaceID: workspaceID})
	}
	_, _, err := a.repo.UpdateBotConfig(ctx, workspaceID, BotConfig{
		WorkspaceID:    workspaceID,
		AutoReply:      config.AutoReply,
		HandoffEnabled: config.HandoffEnabled,
		Tone:           config.Tone,
		WelcomeMessage: config.WelcomeMessage,
		HandoffMessage: config.HandoffMessage,
	}, items)
	return err
}

func (a usecaseRepositoryAdapter) UpdateMasterProfile(ctx context.Context, workspaceID, rawPhone string) (usecase.MasterProfileResult, error) {
	profile, err := a.repo.UpdateMasterProfile(ctx, workspaceID, rawPhone)
	if err != nil {
		return usecase.MasterProfileResult{}, err
	}
	return usecase.MasterProfileResult{
		WorkspaceID:           profile.WorkspaceID,
		MasterPhoneRaw:        profile.MasterPhoneRaw,
		MasterPhoneNormalized: profile.MasterPhoneNormalized,
		TelegramEnabled:       profile.TelegramEnabled,
	}, nil
}

func (a usecaseRepositoryAdapter) SaveChannelSettings(ctx context.Context, input usecase.ChannelAccountInput) (usecase.ChannelAccountResult, error) {
	var (
		account ChannelAccount
		err     error
	)
	if input.EncryptedToken == "" && input.BotUsername == "" && input.WebhookSecret == "" {
		account, err = a.repo.UpdateChannel(ctx, input.WorkspaceID, ChannelProvider(input.Provider), input.Connected, input.Name)
	} else {
		account, err = a.repo.UpsertChannelAccount(ctx, ChannelAccount{
			WorkspaceID:     input.WorkspaceID,
			Provider:        ChannelProvider(input.Provider),
			ChannelKind:     ChannelKind(input.ChannelKind),
			AccountScope:    ChannelAccountScope(input.AccountScope),
			Name:            input.Name,
			Connected:       input.Connected,
			IsEnabled:       input.IsEnabled,
			BotUsername:     input.BotUsername,
			WebhookSecret:   input.WebhookSecret,
			EncryptedToken:  input.EncryptedToken,
			TokenConfigured: input.TokenConfigured,
		})
	}
	if err != nil {
		return usecase.ChannelAccountResult{}, err
	}
	return usecase.ChannelAccountResult{
		ID:              account.ID,
		WorkspaceID:     account.WorkspaceID,
		Provider:        string(account.Provider),
		ChannelKind:     string(account.ChannelKind),
		AccountScope:    string(account.AccountScope),
		Name:            account.Name,
		Connected:       account.Connected,
		IsEnabled:       account.IsEnabled,
		BotUsername:     account.BotUsername,
		TokenConfigured: account.TokenConfigured,
	}, nil
}

func (a usecaseRepositoryAdapter) ReceiveInboundMessage(ctx context.Context, input usecase.InboundInput) (usecase.InboundResult, error) {
	result, err := a.repo.ReceiveInboundMessage(ctx, InboundMessageInput{
		Provider:          ChannelProvider(input.Provider),
		ChannelAccountID:  input.ChannelAccountID,
		ExternalChatID:    input.ExternalChatID,
		ExternalMessageID: input.ExternalMessageID,
		Text:              input.Text,
		Timestamp:         input.Timestamp,
		Profile: InboundProfile{
			Name:     input.Profile.Name,
			Username: input.Profile.Username,
			Phone:    input.Profile.Phone,
		},
	})
	if err != nil {
		return usecase.InboundResult{}, err
	}
	return usecase.InboundResult{
		WorkspaceID:     result.Conversation.WorkspaceID,
		ConversationID:  result.Conversation.ID,
		MessageID:       result.Message.ID,
		CustomerID:      result.Customer.ID,
		ConversationNew: result.Conversation.UnreadCount == 1,
	}, nil
}

func (a usecaseRepositoryAdapter) ReceiveInboundMessageForWorkspace(ctx context.Context, workspaceID string, input usecase.InboundInput) (usecase.InboundResult, error) {
	result, err := a.repo.ReceiveInboundMessageForWorkspace(ctx, workspaceID, InboundMessageInput{
		Provider:          ChannelProvider(input.Provider),
		ChannelAccountID:  input.ChannelAccountID,
		ExternalChatID:    input.ExternalChatID,
		ExternalMessageID: input.ExternalMessageID,
		Text:              input.Text,
		Timestamp:         input.Timestamp,
		Profile: InboundProfile{
			Name:     input.Profile.Name,
			Username: input.Profile.Username,
			Phone:    input.Profile.Phone,
		},
	})
	if err != nil {
		return usecase.InboundResult{}, err
	}
	return usecase.InboundResult{
		WorkspaceID:     result.Conversation.WorkspaceID,
		ConversationID:  result.Conversation.ID,
		MessageID:       result.Message.ID,
		CustomerID:      result.Customer.ID,
		ConversationNew: result.Conversation.UnreadCount == 1,
	}, nil
}

func (a usecaseRepositoryAdapter) LinkOperatorTelegram(ctx context.Context, code, telegramUserID, telegramChatID string) (usecase.OperatorLinkResult, error) {
	binding, err := a.repo.LinkOperatorTelegram(ctx, code, telegramUserID, telegramChatID)
	if err != nil {
		return usecase.OperatorLinkResult{}, err
	}
	return usecase.OperatorLinkResult{
		UserID:         binding.UserID,
		WorkspaceID:    binding.WorkspaceID,
		TelegramUserID: binding.TelegramUserID,
		TelegramChatID: binding.TelegramChatID,
	}, nil
}

func (a usecaseRepositoryAdapter) CreateOperatorLinkCode(ctx context.Context, workspaceID, userID, botUsername string) (usecase.OperatorLinkCodeResult, error) {
	code, err := a.repo.CreateOperatorLinkCode(ctx, workspaceID, userID, botUsername)
	if err != nil {
		return usecase.OperatorLinkCodeResult{}, err
	}
	return usecase.OperatorLinkCodeResult{
		ID:          code.ID,
		UserID:      code.UserID,
		WorkspaceID: code.WorkspaceID,
		Code:        code.Code,
		ExpiresAt:   code.ExpiresAt,
		DeepLink:    code.DeepLink,
	}, nil
}

func (a usecaseRepositoryAdapter) UnlinkOperatorTelegram(ctx context.Context, workspaceID, userID string) error {
	return a.repo.UnlinkOperatorTelegram(ctx, workspaceID, userID)
}

func (a usecaseRepositoryAdapter) SaveBotSession(ctx context.Context, input usecase.BotSessionInput, payload any) (usecase.BotSessionResult, error) {
	session, err := a.repo.SaveBotSession(ctx, BotSession{
		WorkspaceID: input.WorkspaceID,
		BotKind:     ChannelKind(input.BotKind),
		Scope:       BotSessionScope(input.Scope),
		ActorType:   BotSessionActorType(input.ActorType),
		ActorID:     input.ActorID,
		State:       input.State,
		ExpiresAt:   input.ExpiresAt,
	}, payload)
	if err != nil {
		return usecase.BotSessionResult{}, err
	}
	return usecase.BotSessionResult{
		ID:          session.ID,
		WorkspaceID: session.WorkspaceID,
		BotKind:     string(session.BotKind),
		Scope:       string(session.Scope),
		ActorType:   string(session.ActorType),
		ActorID:     session.ActorID,
		State:       session.State,
		Payload:     session.Payload,
		ExpiresAt:   session.ExpiresAt,
		UpdatedAt:   session.UpdatedAt,
	}, nil
}

func (a usecaseRepositoryAdapter) DeleteBotSession(ctx context.Context, workspaceID, scope, actorType, actorID string) error {
	return a.repo.DeleteBotSession(ctx, workspaceID, BotSessionScope(scope), BotSessionActorType(actorType), actorID)
}

func (a usecaseRepositoryAdapter) SaveClientBotRoute(ctx context.Context, input usecase.ClientBotRouteInput) (usecase.ClientBotRouteResult, error) {
	route, err := a.repo.SaveClientBotRoute(ctx, ClientBotRoute{
		ChannelAccountID:              input.ChannelAccountID,
		ExternalChatID:                input.ExternalChatID,
		SelectedWorkspaceID:           input.SelectedWorkspaceID,
		SelectedMasterPhoneNormalized: input.SelectedMasterPhoneNormalized,
		State:                         input.State,
		ExpiresAt:                     input.ExpiresAt,
	})
	if err != nil {
		return usecase.ClientBotRouteResult{}, err
	}
	return usecase.ClientBotRouteResult{
		ChannelAccountID:              route.ChannelAccountID,
		ExternalChatID:                route.ExternalChatID,
		SelectedWorkspaceID:           route.SelectedWorkspaceID,
		SelectedMasterPhoneNormalized: route.SelectedMasterPhoneNormalized,
		State:                         route.State,
		ExpiresAt:                     route.ExpiresAt,
		UpdatedAt:                     route.UpdatedAt,
	}, nil
}

func (a usecaseRepositoryAdapter) ClearClientBotRoute(ctx context.Context, channelAccountID, externalChatID string) error {
	return a.repo.ClearClientBotRoute(ctx, channelAccountID, externalChatID)
}
