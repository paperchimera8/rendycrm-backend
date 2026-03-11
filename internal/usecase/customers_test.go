package usecase

import (
	"context"
	"testing"

	"github.com/vital/rendycrm-app/internal/domain"
)

type customerStoreFake struct {
	customerName string
	auditActions []string
}

func (f *customerStoreFake) UpdateCustomerName(_ context.Context, workspaceID, customerID, name string) (CustomerResult, error) {
	f.customerName = name
	return CustomerResult{ID: customerID, Name: name}, nil
}

func (f *customerStoreFake) AddAuditLog(_ context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	f.auditActions = append(f.auditActions, action)
	return nil
}

func TestCustomerServiceUpdateProfileWritesAudit(t *testing.T) {
	store := &customerStoreFake{}
	service := NewCustomerService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_1", UserID: "user_1"}

	result, err := service.UpdateProfile(context.Background(), actor, "ws_1", "cust_1", "  Anna  ")
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if result.Name != "Anna" || store.customerName != "Anna" {
		t.Fatalf("expected trimmed name, got result=%q store=%q", result.Name, store.customerName)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "customer.updated" {
		t.Fatalf("unexpected audit actions: %#v", store.auditActions)
	}
}
