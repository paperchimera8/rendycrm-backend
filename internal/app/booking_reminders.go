package app

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

const (
	clientBookingReminderLeadTime      = 4 * time.Hour
	clientBookingReminderSweepInterval = time.Minute
	clientBookingReminderBatchSize     = 20
	operatorReminderHorizon            = 7 * 24 * time.Hour
	operatorReminderListLimit          = 10
)

func (s *Server) runClientBookingReminderWorker(ctx context.Context) {
	ticker := time.NewTicker(clientBookingReminderSweepInterval)
	defer ticker.Stop()

	for {
		if err := s.runClientBookingReminderSweep(ctx); err != nil && ctx.Err() == nil {
			log.Printf("client reminder sweep failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) runClientBookingReminderSweep(ctx context.Context) error {
	count, err := s.runtime.repository.EnqueueDueClientTelegramReminders(ctx, time.Now().UTC(), clientBookingReminderLeadTime, clientBookingReminderBatchSize)
	if err != nil {
		return err
	}
	if count > 0 {
		log.Printf("client reminders queued=%d", count)
	}
	return nil
}

func (s *Server) handleOperatorReminderMenu(ctx context.Context, binding OperatorBotBinding) ([]BotOutboundMessage, error) {
	workspace, err := s.runtime.repository.Workspace(ctx, binding.WorkspaceID)
	if err != nil {
		return nil, err
	}
	bookings, err := s.runtime.repository.UpcomingReminderBookings(ctx, binding.WorkspaceID, time.Now().UTC(), operatorReminderHorizon, operatorReminderListLimit)
	if err != nil {
		return nil, err
	}

	lines := []string{"🔔 Напоминания на 7 дней"}
	buttons := make([]string, 0, len(bookings)+1)
	if len(bookings) == 0 {
		lines = append(lines, "", "Подтвержденных записей на ближайшую неделю нет.")
		buttons = append(buttons, "/dashboard")
		return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: strings.Join(lines, "\n"), Buttons: buttons}}, nil
	}

	for idx, booking := range bookings {
		position := idx + 1
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%d. %s - %s", position, formatBookingReminderTime(booking.StartsAt, workspace.Timezone), booking.CustomerName))
		lines = append(lines, "Напоминание: "+bookingReminderStateLabel(booking))
		if booking.ClientReminderSentAt == nil {
			buttons = append(buttons, reminderToggleCallback(booking.ID, booking.ClientReminderEnabled, position))
		}
	}
	buttons = append(buttons, "/dashboard")
	return []BotOutboundMessage{{ChatID: binding.TelegramChatID, Text: strings.Join(lines, "\n"), Buttons: buttons}}, nil
}

func bookingReminderStateLabel(booking Booking) string {
	if booking.ClientReminderSentAt != nil {
		return "отправлено"
	}
	if booking.ClientReminderEnabled {
		return "вкл"
	}
	return "выкл"
}

func formatBookingReminderTime(startsAt time.Time, timezone string) string {
	loc := time.Local
	if trimmed := strings.TrimSpace(timezone); trimmed != "" {
		if loaded, err := time.LoadLocation(trimmed); err == nil {
			loc = loaded
		}
	}
	return startsAt.In(loc).Format("02.01 15:04")
}

func reminderToggleCallback(bookingID string, enabled bool, position int) string {
	action := "on"
	if enabled {
		action = "off"
	}
	return fmt.Sprintf("rmd:toggle:%s:%s:%d", strings.TrimSpace(bookingID), action, position)
}

func parseReminderToggleCommand(command string) (bookingID string, action string, position int, ok bool) {
	parts := strings.Split(strings.TrimSpace(command), ":")
	if len(parts) != 5 || parts[0] != "rmd" || parts[1] != "toggle" {
		return "", "", 0, false
	}
	bookingID = strings.TrimSpace(parts[2])
	action = strings.TrimSpace(parts[3])
	if bookingID == "" || (action != "on" && action != "off") {
		return "", "", 0, false
	}
	position, err := strconv.Atoi(strings.TrimSpace(parts[4]))
	if err != nil || position <= 0 {
		return "", "", 0, false
	}
	return bookingID, action, position, true
}
