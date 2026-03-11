package usecase

import (
	"context"
	"strings"

	"github.com/vital/rendycrm-app/internal/domain"
)

type CustomerResult struct {
	ID   string
	Name string
}

type CustomerStore interface {
	UpdateCustomerName(ctx context.Context, workspaceID, customerID, name string) (CustomerResult, error)
	AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error
}

type CustomerService struct {
	store  CustomerStore
	policy Policy
}

func NewCustomerService(store CustomerStore, policy Policy) CustomerService {
	return CustomerService{store: store, policy: policy}
}

func (s CustomerService) UpdateProfile(ctx context.Context, actor domain.Actor, workspaceID, customerID, name string) (CustomerResult, error) {
	if err := s.policy.CanManageCustomer(actor, workspaceID); err != nil {
		return CustomerResult{}, err
	}
	result, err := s.store.UpdateCustomerName(ctx, workspaceID, customerID, strings.TrimSpace(name))
	if err != nil {
		return CustomerResult{}, err
	}
	_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "customer.updated", "customer", customerID, map[string]any{"source": actor.Kind})
	return result, nil
}
