package usecase

import (
	"context"
	"testing"

	"github.com/vital/rendycrm-app/internal/domain"
)

type dialogStoreFake struct {
	replyResult   DialogReplyResult
	replyCalls    int
	assignCalls   int
	automation    []string
	statusChanges []string
	auditActions  []string
}

func (f *dialogStoreFake) ReplyToDialog(_ context.Context, workspaceID, conversationID, userID, text string) (DialogReplyResult, error) {
	f.replyCalls++
	return f.replyResult, nil
}

func (f *dialogStoreFake) AssignDialog(_ context.Context, workspaceID, conversationID, userID string) error {
	f.assignCalls++
	return nil
}

func (f *dialogStoreFake) SetDialogAutomation(_ context.Context, workspaceID, conversationID, status, intent string) error {
	f.automation = append(f.automation, status+":"+intent)
	return nil
}

func (f *dialogStoreFake) SetDialogStatus(_ context.Context, workspaceID, conversationID, status string) error {
	f.statusChanges = append(f.statusChanges, status)
	return nil
}

func (f *dialogStoreFake) AddAuditLog(_ context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	f.auditActions = append(f.auditActions, action)
	return nil
}

func TestDialogServiceTakeDialogByHumanUsesSinglePath(t *testing.T) {
	store := &dialogStoreFake{}
	service := NewDialogService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: "ws_1", UserID: "user_1"}

	if err := service.TakeDialogByHuman(context.Background(), actor, "conv_1"); err != nil {
		t.Fatalf("take dialog: %v", err)
	}
	if store.assignCalls != 1 {
		t.Fatalf("expected assign call, got %d", store.assignCalls)
	}
	if len(store.automation) != 1 || store.automation[0] != "human:human_request" {
		t.Fatalf("unexpected automation calls: %#v", store.automation)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "dialog.take_human" {
		t.Fatalf("unexpected audit actions: %#v", store.auditActions)
	}
}
