package app

import (
	"context"
	"log"
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

func formatBookingReminderTime(startsAt time.Time, timezone string) string {
	loc := time.Local
	if trimmed := strings.TrimSpace(timezone); trimmed != "" {
		if loaded, err := time.LoadLocation(trimmed); err == nil {
			loc = loaded
		}
	}
	return startsAt.In(loc).Format("02.01 15:04")
}
