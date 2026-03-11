package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
)

type operatorLinkStoreFake struct {
	auditActions []string
}

func (f *operatorLinkStoreFake) LinkOperatorTelegram(_ context.Context, code, telegramUserID, telegramChatID string) (OperatorLinkResult, error) {
	return OperatorLinkResult{UserID: "user_1", WorkspaceID: "ws_1", TelegramUserID: telegramUserID, TelegramChatID: telegramChatID}, nil
}

func (f *operatorLinkStoreFake) CreateOperatorLinkCode(_ context.Context, workspaceID, userID, botUsername string) (OperatorLinkCodeResult, error) {
	return OperatorLinkCodeResult{ID: "code_1", UserID: userID, WorkspaceID: workspaceID, Code: "abc", ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (f *operatorLinkStoreFake) UnlinkOperatorTelegram(_ context.Context, workspaceID, userID string) error {
	return nil
}

func (f *operatorLinkStoreFake) AddAuditLog(_ context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	f.auditActions = append(f.auditActions, action)
	return nil
}

func TestOperatorLinkServiceCreateCodeRequiresUser(t *testing.T) {
	store := &operatorLinkStoreFake{}
	service := NewOperatorLinkService(store, DefaultPolicy{})
	operator := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: "ws_1", UserID: "user_1"}

	if _, err := service.CreateLinkCode(context.Background(), operator, "ws_1", "user_1", "@bot"); err == nil {
		t.Fatalf("expected access denied")
	}
}

func TestOperatorLinkServiceUnlinkWritesAudit(t *testing.T) {
	store := &operatorLinkStoreFake{}
	service := NewOperatorLinkService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_1", UserID: "user_1", Role: string(domain.RoleAdmin)}

	if err := service.UnlinkTelegram(context.Background(), actor, "ws_1", "user_1"); err != nil {
		t.Fatalf("unlink telegram: %v", err)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "operator_bot.unlink" {
		t.Fatalf("unexpected audit actions: %#v", store.auditActions)
	}
}
