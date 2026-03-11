package usecase

import (
	"context"
	"testing"

	"github.com/vital/rendycrm-app/internal/domain"
)

type reviewStoreFake struct {
	status       string
	auditActions []string
}

func (f *reviewStoreFake) UpdateReviewStatus(_ context.Context, workspaceID, reviewID, status string) (ReviewResult, error) {
	f.status = status
	return ReviewResult{ID: reviewID, Status: status}, nil
}

func (f *reviewStoreFake) AddAuditLog(_ context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	f.auditActions = append(f.auditActions, action)
	return nil
}

func TestReviewServiceUpdateStatus(t *testing.T) {
	store := &reviewStoreFake{}
	service := NewReviewService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_1", UserID: "user_1"}

	result, err := service.UpdateStatus(context.Background(), actor, "ws_1", "rev_1", "resolved")
	if err != nil {
		t.Fatalf("update review: %v", err)
	}
	if result.Status != "resolved" || store.status != "resolved" {
		t.Fatalf("unexpected review status: result=%q store=%q", result.Status, store.status)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "review.updated" {
		t.Fatalf("unexpected audit actions: %#v", store.auditActions)
	}
}
