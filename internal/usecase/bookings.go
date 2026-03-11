package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
)

type BookingStore interface {
	CreatePendingBookingForSlot(ctx context.Context, workspaceID, customerID, dailySlotID, notes string) (BookingResult, error)
	CreatePendingBookingForRange(ctx context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, notes string) (BookingResult, error)
	CreateConfirmedBookingForSlot(ctx context.Context, workspaceID, customerID, dailySlotID string, amount int, notes string) (BookingResult, error)
	CreateConfirmedBookingForRange(ctx context.Context, workspaceID, customerID string, startsAt, endsAt time.Time, amount int, notes string) (BookingResult, error)
	ConfirmBooking(ctx context.Context, workspaceID, bookingID string, amount int) (BookingResult, error)
	CancelBooking(ctx context.Context, workspaceID, bookingID string) (BookingResult, error)
	CompleteBooking(ctx context.Context, workspaceID, bookingID string, amount int) (BookingResult, error)
	ReschedulePendingToSlot(ctx context.Context, workspaceID, bookingID, dailySlotID, notes string) (BookingResult, error)
	ReschedulePendingToRange(ctx context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, notes string) (BookingResult, error)
	RescheduleConfirmedToSlot(ctx context.Context, workspaceID, bookingID, dailySlotID string, amount int, notes string) (BookingResult, error)
	RescheduleConfirmedToRange(ctx context.Context, workspaceID, bookingID string, startsAt, endsAt time.Time, amount int, notes string) (BookingResult, error)
	SetDialogAutomation(ctx context.Context, workspaceID, conversationID, status, intent string) error
	AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error
}

type BookingResult struct {
	ID        string
	StartsAt  time.Time
	EndsAt    time.Time
	Status    string
	DailySlot string
}

type CreateBookingInput struct {
	WorkspaceID    string
	CustomerID     string
	DailySlotID    string
	StartsAt       time.Time
	EndsAt         time.Time
	Amount         int
	Status         string
	Notes          string
	ConversationID string
}

type UpdateBookingInput struct {
	WorkspaceID    string
	BookingID      string
	DailySlotID    string
	StartsAt       time.Time
	EndsAt         time.Time
	Amount         int
	Status         string
	Notes          string
	ConversationID string
}

type BookingService struct {
	store  BookingStore
	policy Policy
}

func NewBookingService(store BookingStore, policy Policy) BookingService {
	return BookingService{store: store, policy: policy}
}

func (s BookingService) CreateBooking(ctx context.Context, actor domain.Actor, input CreateBookingInput) (BookingResult, error) {
	if err := s.policy.CanManageBooking(actor, input.WorkspaceID); err != nil {
		return BookingResult{}, err
	}
	var (
		result BookingResult
		err    error
	)
	if input.Status == "confirmed" {
		if input.Amount <= 0 {
			return BookingResult{}, errors.New("amount is required")
		}
		if input.DailySlotID != "" {
			result, err = s.store.CreateConfirmedBookingForSlot(ctx, input.WorkspaceID, input.CustomerID, input.DailySlotID, input.Amount, input.Notes)
		} else {
			result, err = s.store.CreateConfirmedBookingForRange(ctx, input.WorkspaceID, input.CustomerID, input.StartsAt, input.EndsAt, input.Amount, input.Notes)
		}
		if err == nil && input.ConversationID != "" {
			err = s.store.SetDialogAutomation(ctx, input.WorkspaceID, input.ConversationID, "booked", "booking_request")
		}
		if err == nil {
			_ = s.store.AddAuditLog(ctx, input.WorkspaceID, actor.UserID, "booking.confirmed", "booking", result.ID, map[string]any{"source": actor.Kind})
		}
		return result, err
	}
	if input.DailySlotID != "" {
		result, err = s.store.CreatePendingBookingForSlot(ctx, input.WorkspaceID, input.CustomerID, input.DailySlotID, input.Notes)
	} else {
		result, err = s.store.CreatePendingBookingForRange(ctx, input.WorkspaceID, input.CustomerID, input.StartsAt, input.EndsAt, input.Notes)
	}
	return result, err
}

func (s BookingService) ConfirmBooking(ctx context.Context, actor domain.Actor, workspaceID, bookingID string, amount int, conversationID string) (BookingResult, error) {
	if err := s.policy.CanManageBooking(actor, workspaceID); err != nil {
		return BookingResult{}, err
	}
	if amount < 0 {
		return BookingResult{}, errors.New("amount must be non-negative")
	}
	result, err := s.store.ConfirmBooking(ctx, workspaceID, bookingID, amount)
	if err != nil {
		return BookingResult{}, err
	}
	if conversationID != "" {
		if err := s.store.SetDialogAutomation(ctx, workspaceID, conversationID, "booked", "booking_request"); err != nil {
			return BookingResult{}, err
		}
	}
	_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "booking.confirmed", "booking", bookingID, map[string]any{"source": actor.Kind})
	return result, nil
}

func (s BookingService) CompleteBooking(ctx context.Context, actor domain.Actor, workspaceID, bookingID string, amount int) (BookingResult, error) {
	if err := s.policy.CanManageBooking(actor, workspaceID); err != nil {
		return BookingResult{}, err
	}
	if amount <= 0 {
		return BookingResult{}, errors.New("amount is required")
	}
	result, err := s.store.CompleteBooking(ctx, workspaceID, bookingID, amount)
	if err == nil {
		_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "booking.completed", "booking", bookingID, map[string]any{"source": actor.Kind})
	}
	return result, err
}

func (s BookingService) CancelBooking(ctx context.Context, actor domain.Actor, workspaceID, bookingID, conversationID string) (BookingResult, error) {
	if err := s.policy.CanManageBooking(actor, workspaceID); err != nil {
		return BookingResult{}, err
	}
	result, err := s.store.CancelBooking(ctx, workspaceID, bookingID)
	if err != nil {
		return BookingResult{}, err
	}
	if conversationID != "" {
		if err := s.store.SetDialogAutomation(ctx, workspaceID, conversationID, "human", "other"); err != nil {
			return BookingResult{}, err
		}
	}
	_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "booking.cancelled", "booking", bookingID, map[string]any{"source": actor.Kind})
	return result, nil
}

func (s BookingService) RescheduleBooking(ctx context.Context, actor domain.Actor, input UpdateBookingInput) (BookingResult, error) {
	if err := s.policy.CanManageBooking(actor, input.WorkspaceID); err != nil {
		return BookingResult{}, err
	}
	var (
		result BookingResult
		err    error
	)
	if input.Status == "confirmed" {
		if input.Amount <= 0 {
			return BookingResult{}, errors.New("amount is required")
		}
		if input.DailySlotID != "" {
			result, err = s.store.RescheduleConfirmedToSlot(ctx, input.WorkspaceID, input.BookingID, input.DailySlotID, input.Amount, input.Notes)
		} else {
			result, err = s.store.RescheduleConfirmedToRange(ctx, input.WorkspaceID, input.BookingID, input.StartsAt, input.EndsAt, input.Amount, input.Notes)
		}
		if err == nil && input.ConversationID != "" {
			err = s.store.SetDialogAutomation(ctx, input.WorkspaceID, input.ConversationID, "booked", "reschedule")
		}
		if err == nil {
			_ = s.store.AddAuditLog(ctx, input.WorkspaceID, actor.UserID, "booking.rescheduled_confirmed", "booking", input.BookingID, map[string]any{"source": actor.Kind})
		}
		return result, err
	}
	if input.DailySlotID != "" {
		result, err = s.store.ReschedulePendingToSlot(ctx, input.WorkspaceID, input.BookingID, input.DailySlotID, input.Notes)
	} else {
		result, err = s.store.ReschedulePendingToRange(ctx, input.WorkspaceID, input.BookingID, input.StartsAt, input.EndsAt, input.Notes)
	}
	if err == nil {
		_ = s.store.AddAuditLog(ctx, input.WorkspaceID, actor.UserID, "booking.rescheduled_pending", "booking", input.BookingID, map[string]any{"source": actor.Kind})
	}
	return result, err
}
