package usecase

import (
	"context"
	"testing"

	"github.com/vital/rendycrm-app/internal/domain"
)

type botSettingsStoreFake struct {
	config       BotConfigState
	loadCalls    int
	saveCalls    int
	auditActions []string
}

func (f *botSettingsStoreFake) LoadBotConfig(_ context.Context, workspaceID string) (BotConfigState, []FAQItem, error) {
	f.loadCalls++
	return f.config, []FAQItem{{ID: "faq_1", Question: "q", Answer: "a"}}, nil
}

func (f *botSettingsStoreFake) SaveBotConfig(_ context.Context, workspaceID string, config BotConfigState, faq []FAQItem) error {
	f.saveCalls++
	f.config = config
	return nil
}

func (f *botSettingsStoreFake) AddAuditLog(_ context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	f.auditActions = append(f.auditActions, action)
	return nil
}

func TestBotSettingsServiceToggleAutoReply(t *testing.T) {
	store := &botSettingsStoreFake{config: BotConfigState{AutoReply: false}}
	service := NewBotSettingsService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_1", UserID: "user_1", Role: string(domain.RoleAdmin)}

	if err := service.ToggleAutoReply(context.Background(), actor, "ws_1", true); err != nil {
		t.Fatalf("toggle auto reply: %v", err)
	}
	if !store.config.AutoReply {
		t.Fatalf("expected auto reply enabled")
	}
	if store.loadCalls != 1 || store.saveCalls != 1 {
		t.Fatalf("unexpected store calls: load=%d save=%d", store.loadCalls, store.saveCalls)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "settings.auto_reply_enabled" {
		t.Fatalf("unexpected audit actions: %#v", store.auditActions)
	}
}
