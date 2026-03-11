package repository

import (
	"time"

	. "github.com/vital/rendycrm-app/internal/domain"
)

func slotAvailable(start, end time.Time, exceptions []AvailabilityException, bookings []Booking, holds []SlotHold) bool {
	for _, booking := range bookings {
		if booking.Status == BookingCancelled {
			continue
		}
		if start.Before(booking.EndsAt) && end.After(booking.StartsAt) {
			return false
		}
	}
	for _, hold := range holds {
		if hold.ExpiresAt.Before(time.Now().UTC()) {
			continue
		}
		if start.Before(hold.EndsAt) && end.After(hold.StartsAt) {
			return false
		}
	}
	for _, exception := range exceptions {
		if start.Before(exception.EndsAt) && end.After(exception.StartsAt) {
			return false
		}
	}
	return true
}
