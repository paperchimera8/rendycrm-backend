package usecase

import (
	"context"

	"github.com/vital/rendycrm-app/internal/domain"
)

type ReviewResult struct {
	ID     string
	Status string
}

type ReviewStore interface {
	UpdateReviewStatus(ctx context.Context, workspaceID, reviewID, status string) (ReviewResult, error)
	AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error
}

type ReviewService struct {
	store  ReviewStore
	policy Policy
}

func NewReviewService(store ReviewStore, policy Policy) ReviewService {
	return ReviewService{store: store, policy: policy}
}

func (s ReviewService) UpdateStatus(ctx context.Context, actor domain.Actor, workspaceID, reviewID, status string) (ReviewResult, error) {
	if err := s.policy.CanManageReviews(actor, workspaceID); err != nil {
		return ReviewResult{}, err
	}
	result, err := s.store.UpdateReviewStatus(ctx, workspaceID, reviewID, status)
	if err != nil {
		return ReviewResult{}, err
	}
	_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "review.updated", "review", reviewID, map[string]any{"status": status, "source": actor.Kind})
	return result, nil
}
