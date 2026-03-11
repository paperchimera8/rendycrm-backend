package usecase

import (
	"errors"
	"testing"

	"github.com/vital/rendycrm-app/internal/domain"
)

func TestDefaultPolicyDeniesWorkspaceMismatch(t *testing.T) {
	policy := DefaultPolicy{}
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_a", UserID: "user_1"}

	err := policy.CanManageBooking(actor, "ws_b")
	if !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}
}

func TestDefaultPolicyDeniesCustomerDialogReply(t *testing.T) {
	policy := DefaultPolicy{}
	actor := domain.Actor{Kind: domain.ActorCustomerBot, WorkspaceID: "ws_a"}

	err := policy.CanReplyDialog(actor, "ws_a")
	if !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("expected access denied, got %v", err)
	}
}

func TestDefaultPolicyRequiresAdminForChannelManagement(t *testing.T) {
	policy := DefaultPolicy{}
	operator := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_a", UserID: "user_1", Role: string(domain.RoleOperator)}
	if err := policy.CanManageChannels(operator, "ws_a"); !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("expected access denied for non-admin, got %v", err)
	}

	admin := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_a", UserID: "user_2", Role: string(domain.RoleAdmin)}
	if err := policy.CanManageChannels(admin, "ws_a"); err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
}
