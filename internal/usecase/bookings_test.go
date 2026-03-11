package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
)

type bookingStoreFake struct {
	confirmed         BookingResult
	confirmAmount     int
	confirmCalls      int
	automationCalls   []string
	auditActions      []string
	cancelCalls       int
	pendingCreateCall int
}

func (f *bookingStoreFake) CreatePendingBookingForSlot(_ context.Context, workspaceID, customerID, dailySlotID, notes string) (BookingResult, error) {
	f.pendingCreateCall++
	return BookingResult{ID: "booking_pending"}, nil
}

func (f *bookingStoreFake) CreatePendingBookingForRange(_ context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, notes string) (BookingResult, error) {
	f.pendingCreateCall++
	return BookingResult{ID: "booking_pending"}, nil
}

func (f *bookingStoreFake) CreateConfirmedBookingForSlot(_ context.Context, workspaceID, customerID, dailySlotID string, amount int, notes string) (BookingResult, error) {
	return BookingResult{ID: "booking_confirmed"}, nil
}

func (f *bookingStoreFake) CreateConfirmedBookingForRange(_ context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, amount int, notes string) (BookingResult, error) {
	return BookingResult{ID: "booking_confirmed"}, nil
}

func (f *bookingStoreFake) ConfirmBooking(_ context.Context, workspaceID, bookingID string, amount int) (BookingResult, error) {
	f.confirmCalls++
	f.confirmAmount = amount
	return f.confirmed, nil
}

func (f *bookingStoreFake) CancelBooking(_ context.Context, workspaceID, bookingID string) (BookingResult, error) {
	f.cancelCalls++
	return BookingResult{ID: bookingID}, nil
}

func (f *bookingStoreFake) CompleteBooking(_ context.Context, workspaceID, bookingID string, amount int) (BookingResult, error) {
	return BookingResult{ID: bookingID}, nil
}

func (f *bookingStoreFake) ReschedulePendingToSlot(_ context.Context, workspaceID, bookingID, dailySlotID, notes string) (BookingResult, error) {
	return BookingResult{ID: bookingID}, nil
}

func (f *bookingStoreFake) ReschedulePendingToRange(_ context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, notes string) (BookingResult, error) {
	return BookingResult{ID: bookingID}, nil
}

func (f *bookingStoreFake) RescheduleConfirmedToSlot(_ context.Context, workspaceID, bookingID, dailySlotID string, amount int, notes string) (BookingResult, error) {
	return BookingResult{ID: bookingID}, nil
}

func (f *bookingStoreFake) RescheduleConfirmedToRange(_ context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, amount int, notes string) (BookingResult, error) {
	return BookingResult{ID: bookingID}, nil
}

func (f *bookingStoreFake) SetDialogAutomation(_ context.Context, workspaceID, conversationID, status, intent string) error {
	f.automationCalls = append(f.automationCalls, status+":"+intent)
	return nil
}

func (f *bookingStoreFake) AddAuditLog(_ context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	f.auditActions = append(f.auditActions, action)
	return nil
}

func TestBookingServiceConfirmAllowsZeroAmountForClientFlow(t *testing.T) {
	store := &bookingStoreFake{confirmed: BookingResult{ID: "booking_1"}}
	service := NewBookingService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: "ws_1"}

	result, err := service.ConfirmBooking(context.Background(), actor, "ws_1", "booking_1", 0, "conv_1")
	if err != nil {
		t.Fatalf("confirm booking: %v", err)
	}
	if result.ID != "booking_1" {
		t.Fatalf("unexpected booking result: %#v", result)
	}
	if store.confirmCalls != 1 || store.confirmAmount != 0 {
		t.Fatalf("unexpected confirm call state: calls=%d amount=%d", store.confirmCalls, store.confirmAmount)
	}
	if len(store.automationCalls) != 1 || store.automationCalls[0] != "booked:booking_request" {
		t.Fatalf("unexpected automation calls: %#v", store.automationCalls)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "booking.confirmed" {
		t.Fatalf("unexpected audit actions: %#v", store.auditActions)
	}
}
