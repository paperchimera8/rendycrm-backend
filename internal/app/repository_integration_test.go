package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCreateConfirmedBookingMarksWeekAndAvailableSlots(t *testing.T) {
	repo, db, workspaceID, customerID, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	setWorkRule(t, db, workspaceID, 1, 10*60, 15*60)
	loc := mustLocation(t, "Europe/Moscow")
	startsAt := time.Date(2026, 3, 9, 10, 0, 0, 0, loc).UTC()
	endsAt := startsAt.Add(time.Hour)

	before, err := repo.AvailableDaySlots(context.Background(), workspaceID, startsAt.Add(-2*time.Hour), startsAt.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("available slots before booking: %v", err)
	}
	if !containsWindow(before, startsAt, endsAt) {
		t.Fatalf("expected %s-%s to be available before booking", startsAt, endsAt)
	}

	booking, err := repo.CreateConfirmedBooking(context.Background(), workspaceID, customerID, startsAt, endsAt, 4500, "Confirmed")
	if err != nil {
		t.Fatalf("create confirmed booking: %v", err)
	}
	if booking.DailySlotID == "" {
		t.Fatal("expected confirmed booking to get daily_slot_id")
	}

	after, err := repo.AvailableDaySlots(context.Background(), workspaceID, startsAt.Add(-2*time.Hour), startsAt.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("available slots after booking: %v", err)
	}
	if containsWindow(after, startsAt, endsAt) {
		t.Fatalf("expected %s-%s to be removed from available slots", startsAt, endsAt)
	}

	week, err := repo.WeekSlots(context.Background(), workspaceID, startsAt)
	if err != nil {
		t.Fatalf("week slots: %v", err)
	}
	slot := findSlotInWeek(week, startsAt, endsAt)
	if slot == nil {
		t.Fatal("expected booked slot to appear in week slots")
	}
	if slot.Status != DailySlotBooked || slot.CustomerName != "Integration Customer" {
		t.Fatalf("unexpected slot state: status=%s customer=%s", slot.Status, slot.CustomerName)
	}
}

func TestConfirmLegacyPendingBookingMaterializesBookedSlot(t *testing.T) {
	repo, db, workspaceID, customerID, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	loc := mustLocation(t, "Europe/Moscow")
	startsAt := time.Date(2026, 3, 9, 12, 0, 0, 0, loc).UTC()
	endsAt := startsAt.Add(time.Hour)

	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO bookings (id, workspace_id, customer_id, starts_at, ends_at, amount_cents, status, source, notes)
		VALUES ('bok_legacy', $1, $2, $3, $4, 0, 'pending', 'operator', 'Legacy pending')
	`, workspaceID, customerID, startsAt, endsAt); err != nil {
		t.Fatalf("insert legacy booking: %v", err)
	}

	booking, err := repo.UpdateBookingStatus(context.Background(), workspaceID, "bok_legacy", BookingConfirmed, intPtr(5200))
	if err != nil {
		t.Fatalf("confirm legacy booking: %v", err)
	}
	if booking.DailySlotID == "" {
		t.Fatal("expected confirmed legacy booking to get daily_slot_id")
	}

	week, err := repo.WeekSlots(context.Background(), workspaceID, startsAt)
	if err != nil {
		t.Fatalf("week slots: %v", err)
	}
	slot := findSlotInWeek(week, startsAt, endsAt)
	if slot == nil {
		t.Fatal("expected slot after legacy confirm")
	}
	if slot.Status != DailySlotBooked {
		t.Fatalf("expected booked slot after confirm, got %s", slot.Status)
	}
}

func TestRescheduleConfirmedBookingFreesOldWindowAndBooksNewOne(t *testing.T) {
	repo, db, workspaceID, customerID, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	setWorkRule(t, db, workspaceID, 1, 10*60, 16*60)
	loc := mustLocation(t, "Europe/Moscow")
	oldStart := time.Date(2026, 3, 9, 10, 0, 0, 0, loc).UTC()
	oldEnd := oldStart.Add(time.Hour)
	newStart := time.Date(2026, 3, 9, 13, 0, 0, 0, loc).UTC()
	newEnd := newStart.Add(time.Hour)

	booking, err := repo.CreateConfirmedBooking(context.Background(), workspaceID, customerID, oldStart, oldEnd, 4500, "Original")
	if err != nil {
		t.Fatalf("create confirmed booking: %v", err)
	}

	if _, err := repo.RescheduleConfirmedBooking(context.Background(), workspaceID, booking.ID, newStart, newEnd, 4700, "Moved"); err != nil {
		t.Fatalf("reschedule confirmed booking: %v", err)
	}

	available, err := repo.AvailableDaySlots(context.Background(), workspaceID, oldStart.Add(-2*time.Hour), newEnd.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("available slots after reschedule: %v", err)
	}
	if !containsWindow(available, oldStart, oldEnd) {
		t.Fatalf("expected old slot %s-%s to be free again", oldStart, oldEnd)
	}
	if containsWindow(available, newStart, newEnd) {
		t.Fatalf("expected new slot %s-%s to be booked", newStart, newEnd)
	}
}

func TestCancelAndCompleteKeepSlotStateConsistent(t *testing.T) {
	repo, db, workspaceID, customerID, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	setWorkRule(t, db, workspaceID, 1, 9*60, 14*60)
	loc := mustLocation(t, "Europe/Moscow")
	cancelStart := time.Date(2026, 3, 9, 9, 0, 0, 0, loc).UTC()
	cancelEnd := cancelStart.Add(time.Hour)
	completeStart := time.Date(2026, 3, 9, 11, 0, 0, 0, loc).UTC()
	completeEnd := completeStart.Add(time.Hour)

	cancelled, err := repo.CreateConfirmedBooking(context.Background(), workspaceID, customerID, cancelStart, cancelEnd, 4500, "Cancel me")
	if err != nil {
		t.Fatalf("create booking to cancel: %v", err)
	}
	completed, err := repo.CreateConfirmedBooking(context.Background(), workspaceID, customerID, completeStart, completeEnd, 4700, "Complete me")
	if err != nil {
		t.Fatalf("create booking to complete: %v", err)
	}

	if _, err := repo.UpdateBookingStatus(context.Background(), workspaceID, cancelled.ID, BookingCancelled, nil); err != nil {
		t.Fatalf("cancel booking: %v", err)
	}
	if _, err := repo.UpdateBookingStatus(context.Background(), workspaceID, completed.ID, BookingCompleted, intPtr(4700)); err != nil {
		t.Fatalf("complete booking: %v", err)
	}

	available, err := repo.AvailableDaySlots(context.Background(), workspaceID, cancelStart.Add(-time.Hour), completeEnd.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("available slots after cancel/complete: %v", err)
	}
	if !containsWindow(available, cancelStart, cancelEnd) {
		t.Fatalf("expected cancelled slot %s-%s to become available", cancelStart, cancelEnd)
	}
	if containsWindow(available, completeStart, completeEnd) {
		t.Fatalf("expected completed slot %s-%s to stay unavailable", completeStart, completeEnd)
	}

	week, err := repo.WeekSlots(context.Background(), workspaceID, completeStart)
	if err != nil {
		t.Fatalf("week slots: %v", err)
	}
	slot := findSlotInWeek(week, completeStart, completeEnd)
	if slot == nil || slot.Status != DailySlotBooked {
		t.Fatalf("expected completed booking slot to remain booked, got %+v", slot)
	}
}

func TestRepairScheduleConsistencyFixesLegacyState(t *testing.T) {
	repo, db, workspaceID, customerID, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	if _, err := db.ExecContext(context.Background(), `DROP INDEX IF EXISTS idx_daily_slots_workspace_time_unique`); err != nil {
		t.Fatalf("drop unique slot index: %v", err)
	}

	loc := mustLocation(t, "Europe/Moscow")
	startsAt := time.Date(2026, 3, 9, 18, 0, 0, 0, loc).UTC()
	endsAt := startsAt.Add(time.Hour)

	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO daily_slots (id, workspace_id, slot_date, starts_at, ends_at, duration_minutes, position, status, is_manual, note)
		VALUES
			('dsl_dup_a', $1, '2026-03-08', $2, $3, 60, 0, 'booked', TRUE, 'dup a'),
			('dsl_dup_b', $1, '2026-03-08', $2, $3, 60, 1, 'booked', TRUE, 'dup b'),
			('dsl_orphan', $1, '2026-03-09', $4, $5, 60, 2, 'held', TRUE, 'orphan')
	`, workspaceID, startsAt, endsAt, endsAt, endsAt.Add(time.Hour)); err != nil {
		t.Fatalf("insert legacy slots: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO bookings (id, workspace_id, customer_id, starts_at, ends_at, amount_cents, status, source, notes)
		VALUES ('bok_repair', $1, $2, $3, $4, 5000, 'confirmed', 'operator', 'repair me')
	`, workspaceID, customerID, startsAt, endsAt); err != nil {
		t.Fatalf("insert legacy booking: %v", err)
	}

	stats, err := repo.repairScheduleConsistency(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("repair schedule consistency: %v", err)
	}
	if !stats.touched() {
		t.Fatal("expected repair to change legacy data")
	}

	booking, err := repo.Booking(context.Background(), workspaceID, "bok_repair")
	if err != nil {
		t.Fatalf("load repaired booking: %v", err)
	}
	if booking.DailySlotID == "" {
		t.Fatal("expected repaired booking to link to daily slot")
	}

	var duplicateCount int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*)
		FROM daily_slots
		WHERE workspace_id = $1 AND starts_at = $2 AND ends_at = $3
	`, workspaceID, startsAt, endsAt).Scan(&duplicateCount); err != nil {
		t.Fatalf("count repaired duplicates: %v", err)
	}
	if duplicateCount != 1 {
		t.Fatalf("expected duplicate slots to merge, got %d", duplicateCount)
	}

	var orphanStatus string
	if err := db.QueryRowContext(context.Background(), `
		SELECT status
		FROM daily_slots
		WHERE id = 'dsl_orphan' AND workspace_id = $1
	`, workspaceID).Scan(&orphanStatus); err != nil {
		t.Fatalf("load orphan slot: %v", err)
	}
	if orphanStatus != string(DailySlotFree) {
		t.Fatalf("expected orphan slot to become free, got %s", orphanStatus)
	}
}

func TestMasterProfileAndClientBotRoute(t *testing.T) {
	repo, db, workspaceID, _, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()
	seedInboxChannelAccount(t, db, workspaceID, "cha_test_route", ChannelTelegram, "route-secret")

	profile, err := repo.UpdateMasterProfile(context.Background(), workspaceID, "+7 (999) 444-55-66")
	if err != nil {
		t.Fatalf("update master profile: %v", err)
	}
	if profile.MasterPhoneNormalized != "79994445566" {
		t.Fatalf("unexpected normalized phone: %s", profile.MasterPhoneNormalized)
	}

	workspace, err := repo.WorkspaceByMasterPhone(context.Background(), "79994445566")
	if err != nil {
		t.Fatalf("workspace by master phone: %v", err)
	}
	if workspace.ID != workspaceID {
		t.Fatalf("workspace mismatch: got %s want %s", workspace.ID, workspaceID)
	}

	route, err := repo.SaveClientBotRoute(context.Background(), ClientBotRoute{
		ChannelAccountID:              "cha_test_route",
		ExternalChatID:                "chat_42",
		SelectedWorkspaceID:           workspaceID,
		SelectedMasterPhoneNormalized: "79994445566",
		State:                         "ready",
		ExpiresAt:                     time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("save client bot route: %v", err)
	}
	if route.SelectedWorkspaceID != workspaceID {
		t.Fatalf("route workspace mismatch: %s", route.SelectedWorkspaceID)
	}

	loaded, err := repo.ClientBotRouteByChat(context.Background(), "cha_test_route", "chat_42")
	if err != nil {
		t.Fatalf("load client bot route: %v", err)
	}
	if loaded.State != "ready" || loaded.SelectedMasterPhoneNormalized != "79994445566" {
		t.Fatalf("unexpected loaded route: %+v", loaded)
	}

	if err := repo.ClearClientBotRoute(context.Background(), "cha_test_route", "chat_42"); err != nil {
		t.Fatalf("clear client bot route: %v", err)
	}
	if _, err := repo.ClientBotRouteByChat(context.Background(), "cha_test_route", "chat_42"); err == nil {
		t.Fatal("expected cleared route to be missing")
	}
}

func TestReceiveInboundMessageForWorkspaceScopesIdentityByWorkspace(t *testing.T) {
	repo, db, workspaceID, _, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO bot_configs (workspace_id, auto_reply, handoff_enabled, tone)
		VALUES ($1, TRUE, TRUE, 'helpful')
	`, workspaceID); err != nil {
		t.Fatalf("insert first bot config: %v", err)
	}

	secondWorkspaceID := "ws_second"
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO workspaces (id, name, timezone, master_phone_raw, master_phone_normalized)
		VALUES ($1, 'Second Workspace', 'Europe/Moscow', '+7 999 222-33-44', '79992223344')
	`, secondWorkspaceID); err != nil {
		t.Fatalf("insert second workspace: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO bot_configs (workspace_id, auto_reply, handoff_enabled, tone)
		VALUES ($1, TRUE, TRUE, 'helpful')
	`, secondWorkspaceID); err != nil {
		t.Fatalf("insert second bot config: %v", err)
	}
	seedInboxChannelAccount(t, db, secondWorkspaceID, "cha_second_tg", ChannelTelegram, "second-secret")

	globalAccountID := "cha_global_tg_test"
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO channel_accounts (id, workspace_id, provider, channel_kind, account_scope, account_name, external_account_id, webhook_secret, connected, is_enabled, bot_username)
		VALUES ($1, 'ws_system', 'telegram', 'telegram_client', 'global', 'Global Telegram', 'telegram_client', 'global-secret', TRUE, TRUE, 'global_test_bot')
	`, globalAccountID); err != nil {
		t.Fatalf("insert global channel account: %v", err)
	}

	input := InboundMessageInput{
		Provider:          ChannelTelegram,
		ChannelAccountID:  globalAccountID,
		ExternalChatID:    "tg-chat-1",
		ExternalMessageID: "msg-1",
		Text:              "Есть ли окно завтра?",
		Timestamp:         time.Now().UTC(),
		Profile:           InboundProfile{Name: "Client One", Username: "client_one"},
	}
	first, err := repo.ReceiveInboundMessageForWorkspace(context.Background(), workspaceID, input)
	if err != nil {
		t.Fatalf("receive for first workspace: %v", err)
	}
	second, err := repo.ReceiveInboundMessageForWorkspace(context.Background(), secondWorkspaceID, InboundMessageInput{
		Provider:          ChannelTelegram,
		ChannelAccountID:  globalAccountID,
		ExternalChatID:    "tg-chat-1",
		ExternalMessageID: "msg-2",
		Text:              "Хочу запись",
		Timestamp:         time.Now().UTC(),
		Profile:           InboundProfile{Name: "Client One", Username: "client_one"},
	})
	if err != nil {
		t.Fatalf("receive for second workspace: %v", err)
	}
	if first.Conversation.WorkspaceID != workspaceID || second.Conversation.WorkspaceID != secondWorkspaceID {
		t.Fatalf("unexpected conversation routing: first=%s second=%s", first.Conversation.WorkspaceID, second.Conversation.WorkspaceID)
	}
	if first.Customer.ID == second.Customer.ID {
		t.Fatal("expected workspace-scoped customer identities for the same external chat")
	}
}

func TestTimezoneAwareSlotDateUsesWorkspaceTimezone(t *testing.T) {
	repo, _, workspaceID, customerID, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	loc := mustLocation(t, "Europe/Moscow")
	startsAt := time.Date(2026, 3, 10, 0, 30, 0, 0, loc).UTC()
	endsAt := startsAt.Add(time.Hour)

	booking, err := repo.CreateConfirmedBooking(context.Background(), workspaceID, customerID, startsAt, endsAt, 6000, "Midnight")
	if err != nil {
		t.Fatalf("create timezone-aware booking: %v", err)
	}

	slot, err := repo.DailySlot(context.Background(), workspaceID, booking.DailySlotID)
	if err != nil {
		t.Fatalf("load timezone-aware slot: %v", err)
	}
	if slot.SlotDate != "2026-03-10" {
		t.Fatalf("expected local slot date 2026-03-10, got %s", slot.SlotDate)
	}

	daySlots, err := repo.DaySlots(context.Background(), workspaceID, startsAt)
	if err != nil {
		t.Fatalf("day slots by UTC instant: %v", err)
	}
	if !containsWindow(daySlots, startsAt, endsAt) {
		t.Fatalf("expected slot %s-%s in timezone-aware day listing", startsAt, endsAt)
	}
}

func TestReceiveInboundMessageAutoRepliesAndDeduplicates(t *testing.T) {
	repo, db, workspaceID, _, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	seedInboxChannelAccount(t, db, workspaceID, "cha_tg", ChannelTelegram, "telegram-secret")
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO bot_configs (workspace_id, auto_reply, handoff_enabled, tone)
		VALUES ($1, TRUE, TRUE, 'helpful')
	`, workspaceID); err != nil {
		t.Fatalf("seed bot config: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO faq_items (id, workspace_id, question, answer)
		VALUES ('faq_price', $1, 'Сколько стоит покрытие?', 'Покрытие стоит 5000 ₽.')
	`, workspaceID); err != nil {
		t.Fatalf("seed faq: %v", err)
	}

	result, err := repo.ReceiveInboundMessage(context.Background(), InboundMessageInput{
		Provider:          ChannelTelegram,
		ChannelAccountID:  "cha_tg",
		ExternalChatID:    "tg-chat-1",
		ExternalMessageID: "msg-ext-1",
		Text:              "Сколько стоит покрытие?",
		Timestamp:         time.Now().UTC(),
		Profile: InboundProfile{
			Name:     "Telegram Lead",
			Username: "tg_lead",
		},
	})
	if err != nil {
		t.Fatalf("receive inbound: %v", err)
	}
	if result.Conversation.Status != ConversationAuto {
		t.Fatalf("expected auto conversation status, got %s", result.Conversation.Status)
	}
	if result.Conversation.Intent != IntentFAQ {
		t.Fatalf("expected faq intent, got %s", result.Conversation.Intent)
	}
	if len(result.Responses) != 1 || result.Responses[0].Text != "Покрытие стоит 5000 ₽." {
		t.Fatalf("unexpected auto response: %+v", result.Responses)
	}

	_, err = repo.ReceiveInboundMessage(context.Background(), InboundMessageInput{
		Provider:          ChannelTelegram,
		ChannelAccountID:  "cha_tg",
		ExternalChatID:    "tg-chat-1",
		ExternalMessageID: "msg-ext-1",
		Text:              "Сколько стоит покрытие?",
		Timestamp:         time.Now().UTC(),
		Profile:           InboundProfile{Name: "Telegram Lead", Username: "tg_lead"},
	})
	if err != nil {
		t.Fatalf("receive duplicate inbound: %v", err)
	}

	_, messages, _, err := repo.ConversationDetail(context.Background(), workspaceID, result.Conversation.ID)
	if err != nil {
		t.Fatalf("load conversation detail: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages after duplicate inbound (1 inbound + 1 bot), got %d", len(messages))
	}
	if messages[1].DeliveryStatus != string(MessageDeliveryQueued) {
		t.Fatalf("expected queued bot message delivery status, got %s", messages[1].DeliveryStatus)
	}
	var outboundCount int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*)
		FROM outbound_messages
		WHERE conversation_id = $1
	`, result.Conversation.ID).Scan(&outboundCount); err != nil {
		t.Fatalf("count outbound messages: %v", err)
	}
	if outboundCount != 1 {
		t.Fatalf("expected 1 outbound queue row, got %d", outboundCount)
	}
}

func TestOperatorBotLinkLifecycle(t *testing.T) {
	repo, db, workspaceID, _, cleanup := newIntegrationRepository(t, "Europe/Moscow")
	defer cleanup()

	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO users (id, email, password_hash, name, status)
		VALUES ('usr_test', 'operator@test.local', $1, 'Operator', 'active')
	`, hashToken("password")); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO workspace_members (id, workspace_id, user_id, role)
		VALUES ('wsm_test', $1, 'usr_test', 'admin')
	`, workspaceID); err != nil {
		t.Fatalf("seed workspace member: %v", err)
	}

	link, err := repo.CreateOperatorLinkCode(context.Background(), workspaceID, "usr_test", "rendycrm_operator_bot")
	if err != nil {
		t.Fatalf("create operator link code: %v", err)
	}
	if link.Code == "" || link.DeepLink == "" {
		t.Fatalf("expected link code and deep link, got %+v", link)
	}

	binding, err := repo.LinkOperatorTelegram(context.Background(), link.Code, "tg-user-1", "tg-chat-1")
	if err != nil {
		t.Fatalf("link operator telegram: %v", err)
	}
	if binding.TelegramChatID != "tg-chat-1" {
		t.Fatalf("unexpected binding: %+v", binding)
	}

	settings, err := repo.OperatorBotSettings(context.Background(), workspaceID, "usr_test", "rendycrm_operator_bot", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("operator bot settings: %v", err)
	}
	if settings.Binding == nil || settings.Binding.TelegramChatID != "tg-chat-1" {
		t.Fatalf("expected active binding in settings, got %+v", settings.Binding)
	}

	if _, err := repo.LinkOperatorTelegram(context.Background(), link.Code, "tg-user-1", "tg-chat-1"); err == nil {
		t.Fatalf("expected reused link code to fail")
	}

	if err := repo.UnlinkOperatorTelegram(context.Background(), workspaceID, "usr_test"); err != nil {
		t.Fatalf("unlink operator telegram: %v", err)
	}
	settings, err = repo.OperatorBotSettings(context.Background(), workspaceID, "usr_test", "rendycrm_operator_bot", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("operator bot settings after unlink: %v", err)
	}
	if settings.Binding != nil {
		t.Fatalf("expected operator binding to be removed after unlink, got %+v", settings.Binding)
	}
}

func newIntegrationRepository(t *testing.T, timezone string) (*Repository, *sql.DB, string, string, func()) {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	adminDSN := os.Getenv("TEST_POSTGRES_ADMIN_DSN")
	if adminDSN == "" {
		adminDSN = "postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable"
	}
	adminDB, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	defer adminDB.Close()

	dbName := fmt.Sprintf("rendycrm_it_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(context.Background(), fmt.Sprintf(`CREATE DATABASE %s`, dbName)); err != nil {
		t.Fatalf("create test db: %v", err)
	}

	dbDSN := strings.Replace(adminDSN, "/postgres?", "/"+dbName+"?", 1)
	if !strings.Contains(dbDSN, "/"+dbName+"?") {
		dbDSN = strings.TrimSuffix(adminDSN, "/postgres") + "/" + dbName + "?sslmode=disable"
	}
	db, err := sql.Open("pgx", dbDSN)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	migrationsPath := migrationsPathFromThisFile(t)
	if err := runMigration(context.Background(), db, migrationsPath); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	workspaceID := "ws_test"
	customerID := "cus_test"
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO workspaces (id, name, timezone) VALUES ($1, 'Integration Workspace', $2)
	`, workspaceID, timezone); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO customers (id, workspace_id, name, notes) VALUES ($1, $2, 'Integration Customer', '')
	`, customerID, workspaceID); err != nil {
		t.Fatalf("seed workspace/customer: %v", err)
	}

	repo := NewRepository(db)
	if err := repo.EnsureSlotSystem(context.Background(), workspaceID); err != nil {
		t.Fatalf("ensure slot system: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		admin, err := sql.Open("pgx", adminDSN)
		if err != nil {
			return
		}
		defer admin.Close()
		_, _ = admin.ExecContext(context.Background(), fmt.Sprintf(`DROP DATABASE %s WITH (FORCE)`, dbName))
	}
	return repo, db, workspaceID, customerID, cleanup
}

func migrationsPathFromThisFile(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations"))
}

func setWorkRule(t *testing.T, db *sql.DB, workspaceID string, dayOfWeek, startMinute, endMinute int) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO availability_rules (id, workspace_id, day_of_week, start_minute, end_minute, enabled)
		VALUES ($1, $2, $3, $4, $5, TRUE)
	`, fmt.Sprintf("avr_%d_%d", dayOfWeek, time.Now().UnixNano()), workspaceID, dayOfWeek, startMinute, endMinute); err != nil {
		t.Fatalf("insert availability rule: %v", err)
	}
}

func mustLocation(t *testing.T, timezone string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatalf("load location %s: %v", timezone, err)
	}
	return loc
}

func containsWindow(slots []DailySlot, startsAt, endsAt time.Time) bool {
	for _, slot := range slots {
		if slot.StartsAt.Equal(startsAt) && slot.EndsAt.Equal(endsAt) {
			return true
		}
	}
	return false
}

func findSlotInWeek(days []WeekSlotDay, startsAt, endsAt time.Time) *DailySlot {
	for _, day := range days {
		for _, slot := range day.Slots {
			if slot.StartsAt.Equal(startsAt) && slot.EndsAt.Equal(endsAt) {
				match := slot
				return &match
			}
		}
	}
	return nil
}

func intPtr(value int) *int {
	return &value
}

func seedInboxChannelAccount(t *testing.T, db *sql.DB, workspaceID, accountID string, provider ChannelProvider, secret string) {
	t.Helper()
	kind := defaultChannelKind(provider)
	botUsername := ""
	if kind == ChannelKindTelegramClient {
		botUsername = "test_client_bot"
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO channel_accounts (id, workspace_id, provider, channel_kind, account_name, external_account_id, webhook_secret, connected, is_enabled, bot_username)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE, TRUE, $8)
	`, accountID, workspaceID, provider, kind, string(provider)+" account", string(provider)+"-ext", secret, botUsername); err != nil {
		t.Fatalf("seed channel account: %v", err)
	}
}
