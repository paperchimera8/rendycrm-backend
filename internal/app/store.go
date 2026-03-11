package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu                     sync.RWMutex
	users                  map[string]User
	workspaces             map[string]Workspace
	sessions               map[string]Session
	customers              map[string]Customer
	channelAccounts        map[ChannelProvider]ChannelAccount
	conversations          map[string]Conversation
	conversationMessages   map[string][]Message
	availabilityRules      []AvailabilityRule
	availabilityExceptions []AvailabilityException
	slotHolds              map[string]SlotHold
	bookings               map[string]Booking
	reviews                map[string]Review
	botConfig              BotConfig
	faqItems               []FAQItem
	seq                    int
}

func NewStore() *Store {
	now := time.Now().UTC()
	workspace := Workspace{ID: "ws_1", Name: "Rendy CRM"}
	operator := User{ID: "usr_1", Email: envOrDefault("DEV_STORE_OPERATOR_EMAIL", ""), Name: "Main Operator", Password: envOrDefault("DEV_STORE_OPERATOR_PASSWORD", ""), Role: RoleAdmin}
	customer := Customer{
		ID:               "cus_1",
		WorkspaceID:      workspace.ID,
		Name:             "Anna Petrova",
		Phone:            "+79001234567",
		Email:            "anna@example.com",
		Notes:            "Prefers evening appointments",
		LastVisitAt:      now.AddDate(0, 0, -14),
		BookingCount:     3,
		PreferredChannel: string(ChannelTelegram),
	}
	conversation := Conversation{
		ID:              "cnv_1",
		WorkspaceID:     workspace.ID,
		CustomerID:      customer.ID,
		Provider:        ChannelTelegram,
		Title:           customer.Name,
		Status:          ConversationOpen,
		AssignedUserID:  operator.ID,
		UnreadCount:     1,
		AISummary:       "Клиент хочет перенести запись на более позднее время вечером.",
		LastMessageText: "Можно на пятницу после 18:00?",
		UpdatedAt:       now.Add(-15 * time.Minute),
	}
	messages := []Message{
		{
			ID:             "msg_1",
			ConversationID: conversation.ID,
			Direction:      MessageInbound,
			SenderType:     MessageSenderCustomer,
			Text:           "Можно на пятницу после 18:00?",
			Status:         "received",
			CreatedAt:      now.Add(-15 * time.Minute),
		},
	}
	booking := Booking{
		ID:          "bok_1",
		WorkspaceID: workspace.ID,
		CustomerID:  customer.ID,
		StartsAt:    time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, time.UTC),
		EndsAt:      time.Date(now.Year(), now.Month(), now.Day(), 19, 0, 0, 0, time.UTC),
		Status:      BookingPending,
		Source:      "operator",
		Notes:       "Перенос по запросу из диалога",
	}
	review := Review{
		ID:          "rev_1",
		WorkspaceID: workspace.ID,
		CustomerID:  customer.ID,
		BookingID:   booking.ID,
		Rating:      4,
		Text:        "Все понравилось, но пришлось подождать 10 минут.",
		Status:      ReviewOpen,
		CreatedAt:   now.AddDate(0, 0, -1),
	}
	return &Store{
		users:      map[string]User{operator.ID: operator},
		workspaces: map[string]Workspace{workspace.ID: workspace},
		sessions:   map[string]Session{},
		customers:  map[string]Customer{customer.ID: customer},
		channelAccounts: map[ChannelProvider]ChannelAccount{
			ChannelTelegram: {ID: "cha_1", WorkspaceID: workspace.ID, Provider: ChannelTelegram, Name: "Telegram salon", Connected: true, WebhookURL: "/webhooks/telegram"},
			ChannelWhatsApp: {ID: "cha_2", WorkspaceID: workspace.ID, Provider: ChannelWhatsApp, Name: "WhatsApp salon", Connected: false, WebhookURL: "/webhooks/whatsapp"},
		},
		conversations:        map[string]Conversation{conversation.ID: conversation},
		conversationMessages: map[string][]Message{conversation.ID: messages},
		availabilityRules: []AvailabilityRule{
			{ID: "avr_1", WorkspaceID: workspace.ID, DayOfWeek: 1, StartMinute: 9 * 60, EndMinute: 18 * 60, Enabled: true},
			{ID: "avr_2", WorkspaceID: workspace.ID, DayOfWeek: 2, StartMinute: 9 * 60, EndMinute: 18 * 60, Enabled: true},
			{ID: "avr_3", WorkspaceID: workspace.ID, DayOfWeek: 3, StartMinute: 9 * 60, EndMinute: 18 * 60, Enabled: true},
			{ID: "avr_4", WorkspaceID: workspace.ID, DayOfWeek: 4, StartMinute: 11 * 60, EndMinute: 20 * 60, Enabled: true},
			{ID: "avr_5", WorkspaceID: workspace.ID, DayOfWeek: 5, StartMinute: 11 * 60, EndMinute: 20 * 60, Enabled: true},
		},
		availabilityExceptions: []AvailabilityException{},
		slotHolds:              map[string]SlotHold{},
		bookings:               map[string]Booking{booking.ID: booking},
		reviews:                map[string]Review{review.ID: review},
		botConfig:              BotConfig{WorkspaceID: workspace.ID, AutoReply: true, HandoffEnabled: true, FAQCount: 2, Tone: "helpful"},
		faqItems: []FAQItem{
			{ID: "faq_1", WorkspaceID: workspace.ID, Question: "Какие есть окна вечером?", Answer: "Проверяю доступность и предлагаю ближайшие слоты."},
			{ID: "faq_2", WorkspaceID: workspace.ID, Question: "Как отменить запись?", Answer: "Могу отменить запись и предложить новое время."},
		},
		seq: 10,
	}
}

func (s *Store) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s_%d", prefix, s.seq)
}

func (s *Store) Login(email, password string) (Session, User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, user := range s.users {
		if strings.EqualFold(user.Email, email) && user.Password == password {
			token := s.nextID("sess")
			session := Session{Token: token, UserID: user.ID, WorkspaceID: "ws_1", ExpiresAt: time.Now().UTC().Add(24 * time.Hour)}
			s.sessions[token] = session
			return session, user, nil
		}
	}
	return Session{}, User{}, errors.New("invalid credentials")
}

func (s *Store) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *Store) Session(token string) (Session, User, Workspace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[token]
	if !ok || session.ExpiresAt.Before(time.Now().UTC()) {
		return Session{}, User{}, Workspace{}, false
	}
	user, ok := s.users[session.UserID]
	if !ok {
		return Session{}, User{}, Workspace{}, false
	}
	workspace, ok := s.workspaces[session.WorkspaceID]
	if !ok {
		return Session{}, User{}, Workspace{}, false
	}
	return session, user, workspace, true
}

func (s *Store) Dashboard() Dashboard {
	s.mu.RLock()
	defer s.mu.RUnlock()
	today := time.Now().UTC().Format("2006-01-02")
	var dashboard Dashboard
	for _, booking := range s.bookings {
		if booking.StartsAt.UTC().Format("2006-01-02") == today {
			dashboard.TodayBookings++
		}
		switch booking.Status {
		case BookingPending:
			dashboard.AwaitingConfirmation++
		case BookingCancelled:
			dashboard.CancelledBookings++
		}
	}
	for _, conversation := range s.conversations {
		dashboard.NewMessages += conversation.UnreadCount
		if conversation.Status == ConversationOpen {
			dashboard.ActiveConversations++
		}
	}
	for _, review := range s.reviews {
		if review.Status == ReviewOpen {
			dashboard.NewReviews++
		}
	}
	return dashboard
}

func (s *Store) Conversations() []Conversation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Conversation, 0, len(s.conversations))
	for _, conversation := range s.conversations {
		items = append(items, conversation)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

func (s *Store) Conversation(id string) (Conversation, []Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conversation, ok := s.conversations[id]
	if !ok {
		return Conversation{}, nil, false
	}
	messages := append([]Message(nil), s.conversationMessages[id]...)
	return conversation, messages, true
}

func (s *Store) Reply(conversationID, assignedUserID, text string) (Conversation, Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, ok := s.conversations[conversationID]
	if !ok {
		return Conversation{}, Message{}, errors.New("conversation not found")
	}
	message := Message{
		ID:             s.nextID("msg"),
		ConversationID: conversationID,
		Direction:      MessageOutbound,
		SenderType:     MessageSenderOperator,
		Text:           text,
		Status:         "sent",
		CreatedAt:      time.Now().UTC(),
	}
	conversation.LastMessageText = text
	conversation.AssignedUserID = assignedUserID
	conversation.UpdatedAt = message.CreatedAt
	conversation.UnreadCount = 0
	conversation.Status = ConversationOpen
	s.conversations[conversationID] = conversation
	messages := append(s.conversationMessages[conversationID], message)
	s.conversationMessages[conversationID] = messages
	return conversation, message, nil
}

func (s *Store) AssignConversation(conversationID, userID string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversation, ok := s.conversations[conversationID]
	if !ok {
		return Conversation{}, errors.New("conversation not found")
	}
	conversation.AssignedUserID = userID
	conversation.UpdatedAt = time.Now().UTC()
	s.conversations[conversationID] = conversation
	return conversation, nil
}

func (s *Store) SimulateInbound(provider ChannelProvider, customerName, text string) (Conversation, Message, Customer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	customer := Customer{
		ID:               s.nextID("cus"),
		WorkspaceID:      "ws_1",
		Name:             customerName,
		Phone:            "+79000000000",
		Email:            "",
		Notes:            "Created from webhook",
		PreferredChannel: string(provider),
	}
	s.customers[customer.ID] = customer
	conversation := Conversation{
		ID:              s.nextID("cnv"),
		WorkspaceID:     "ws_1",
		CustomerID:      customer.ID,
		Provider:        provider,
		Title:           customer.Name,
		Status:          ConversationOpen,
		UnreadCount:     1,
		AISummary:       "Новый входящий лид из канала.",
		LastMessageText: text,
		UpdatedAt:       time.Now().UTC(),
	}
	message := Message{
		ID:             s.nextID("msg"),
		ConversationID: conversation.ID,
		Direction:      MessageInbound,
		SenderType:     MessageSenderCustomer,
		Text:           text,
		Status:         "received",
		CreatedAt:      conversation.UpdatedAt,
	}
	s.conversations[conversation.ID] = conversation
	s.conversationMessages[conversation.ID] = []Message{message}
	return conversation, message, customer
}

func (s *Store) Customer(id string) (Customer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	customer, ok := s.customers[id]
	return customer, ok
}

func (s *Store) Availability() ([]AvailabilityRule, []AvailabilityException, []Slot) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules := append([]AvailabilityRule(nil), s.availabilityRules...)
	exceptions := append([]AvailabilityException(nil), s.availabilityExceptions...)
	slots := s.availableSlotsLocked(time.Now().UTC())
	return rules, exceptions, slots
}

func (s *Store) UpdateAvailabilityRules(rules []AvailabilityRule) []AvailabilityRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range rules {
		if rules[i].ID == "" {
			rules[i].ID = s.nextID("avr")
		}
		rules[i].WorkspaceID = "ws_1"
	}
	s.availabilityRules = rules
	return append([]AvailabilityRule(nil), s.availabilityRules...)
}

func (s *Store) UpdateAvailabilityExceptions(exceptions []AvailabilityException) []AvailabilityException {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range exceptions {
		if exceptions[i].ID == "" {
			exceptions[i].ID = s.nextID("avx")
		}
		exceptions[i].WorkspaceID = "ws_1"
	}
	s.availabilityExceptions = exceptions
	return append([]AvailabilityException(nil), s.availabilityExceptions...)
}

func (s *Store) CreateBooking(customerID string, startsAt, endsAt time.Time, notes string) (Booking, SlotHold, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.customers[customerID]; !ok {
		return Booking{}, SlotHold{}, errors.New("customer not found")
	}
	if !s.slotAvailableLocked(startsAt, endsAt) {
		return Booking{}, SlotHold{}, errors.New("slot unavailable")
	}
	hold := SlotHold{ID: s.nextID("hold"), WorkspaceID: "ws_1", CustomerID: customerID, StartsAt: startsAt, EndsAt: endsAt, ExpiresAt: time.Now().UTC().Add(15 * time.Minute)}
	s.slotHolds[hold.ID] = hold
	booking := Booking{ID: s.nextID("bok"), WorkspaceID: "ws_1", CustomerID: customerID, StartsAt: startsAt, EndsAt: endsAt, Status: BookingPending, Source: "operator", Notes: notes}
	s.bookings[booking.ID] = booking
	return booking, hold, nil
}

func (s *Store) ConfirmBooking(id string) (Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	booking, ok := s.bookings[id]
	if !ok {
		return Booking{}, errors.New("booking not found")
	}
	booking.Status = BookingConfirmed
	s.bookings[id] = booking
	return booking, nil
}

func (s *Store) CancelBooking(id string) (Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	booking, ok := s.bookings[id]
	if !ok {
		return Booking{}, errors.New("booking not found")
	}
	booking.Status = BookingCancelled
	s.bookings[id] = booking
	return booking, nil
}

func (s *Store) Reviews() []Review {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Review, 0, len(s.reviews))
	for _, review := range s.reviews {
		items = append(items, review)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

func (s *Store) UpdateReviewStatus(id string, status ReviewStatus) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	review, ok := s.reviews[id]
	if !ok {
		return Review{}, errors.New("review not found")
	}
	review.Status = status
	s.reviews[id] = review
	return review, nil
}

func (s *Store) Analytics() AnalyticsOverview {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return AnalyticsOverview{
		Revenue:               len(s.bookings) * 4500,
		ConfirmationRate:      82,
		NoShowRate:            7,
		RepeatBookings:        46,
		ConversationToBooking: 34,
	}
}

func (s *Store) ChannelAccounts() []ChannelAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]ChannelAccount, 0, len(s.channelAccounts))
	for _, account := range s.channelAccounts {
		items = append(items, account)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Provider < items[j].Provider })
	return items
}

func (s *Store) UpdateChannel(provider ChannelProvider, connected bool, name string) (ChannelAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account, ok := s.channelAccounts[provider]
	if !ok {
		return ChannelAccount{}, errors.New("channel not found")
	}
	account.Connected = connected
	if name != "" {
		account.Name = name
	}
	s.channelAccounts[provider] = account
	return account, nil
}

func (s *Store) BotConfig() (BotConfig, []FAQItem) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.botConfig, append([]FAQItem(nil), s.faqItems...)
}

func (s *Store) UpdateBotConfig(config BotConfig, faqItems []FAQItem) (BotConfig, []FAQItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config.WorkspaceID = "ws_1"
	config.FAQCount = len(faqItems)
	s.botConfig = config
	for i := range faqItems {
		if faqItems[i].ID == "" {
			faqItems[i].ID = s.nextID("faq")
		}
		faqItems[i].WorkspaceID = "ws_1"
	}
	s.faqItems = faqItems
	return s.botConfig, append([]FAQItem(nil), s.faqItems...)
}

func (s *Store) availableSlotsLocked(base time.Time) []Slot {
	slots := make([]Slot, 0, 6)
	for dayOffset := 0; dayOffset < 7 && len(slots) < 6; dayOffset++ {
		current := base.AddDate(0, 0, dayOffset)
		weekday := int(current.Weekday())
		for _, rule := range s.availabilityRules {
			if !rule.Enabled || rule.DayOfWeek != weekday {
				continue
			}
			start := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(rule.StartMinute) * time.Minute)
			end := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(rule.EndMinute) * time.Minute)
			for slotStart := start; slotStart.Add(time.Hour).Before(end) || slotStart.Add(time.Hour).Equal(end); slotStart = slotStart.Add(2 * time.Hour) {
				slotEnd := slotStart.Add(time.Hour)
				if s.slotAvailableLocked(slotStart, slotEnd) {
					slots = append(slots, Slot{Start: slotStart, End: slotEnd})
					if len(slots) >= 6 {
						return slots
					}
				}
			}
		}
	}
	return slots
}

func (s *Store) slotAvailableLocked(start, end time.Time) bool {
	for _, booking := range s.bookings {
		if booking.Status == BookingCancelled {
			continue
		}
		if start.Before(booking.EndsAt) && end.After(booking.StartsAt) {
			return false
		}
	}
	for _, hold := range s.slotHolds {
		if hold.ExpiresAt.Before(time.Now().UTC()) {
			continue
		}
		if start.Before(hold.EndsAt) && end.After(hold.StartsAt) {
			return false
		}
	}
	for _, exception := range s.availabilityExceptions {
		if start.Before(exception.EndsAt) && end.After(exception.StartsAt) {
			return false
		}
	}
	return true
}
