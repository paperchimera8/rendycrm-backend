package app

import "time"

func computeAvailableSlots(base time.Time, rules []AvailabilityRule, exceptions []AvailabilityException, bookings []Booking, holds []SlotHold) []Slot {
	slots := make([]Slot, 0, 6)
	for dayOffset := 0; dayOffset < 7 && len(slots) < 6; dayOffset++ {
		current := base.AddDate(0, 0, dayOffset)
		weekday := int(current.Weekday())
		for _, rule := range rules {
			if !rule.Enabled || rule.DayOfWeek != weekday {
				continue
			}
			start := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(rule.StartMinute) * time.Minute)
			end := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(rule.EndMinute) * time.Minute)
			for slotStart := start; slotStart.Add(time.Hour).Before(end) || slotStart.Add(time.Hour).Equal(end); slotStart = slotStart.Add(time.Hour) {
				slotEnd := slotStart.Add(time.Hour)
				if slotAvailable(slotStart, slotEnd, exceptions, bookings, holds) {
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
