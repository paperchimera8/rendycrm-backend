package app

import (
	"strings"
	"testing"
	"time"
)

func TestFormatSlotsForBotShowsTimeInterval(t *testing.T) {
	start := time.Date(2026, 3, 20, 15, 30, 0, 0, time.Local)
	end := start.Add(90 * time.Minute)

	text, buttons := formatSlotsForBot([]DailySlot{{
		ID:       "dsl_1",
		StartsAt: start,
		EndsAt:   end,
		Status:   DailySlotFree,
	}})

	if !strings.Contains(text, "20.03 15:30-17:00") {
		t.Fatalf("expected interval in bot text, got %q", text)
	}
	if len(buttons) != 1 || buttons[0] != "20.03 15:30-17:00" {
		t.Fatalf("unexpected bot buttons: %#v", buttons)
	}
}
