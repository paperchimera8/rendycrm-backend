package usecase

import (
	"context"
	"testing"

	"github.com/vital/rendycrm-app/internal/domain"
)

type channelStoreFake struct {
	profile      MasterProfileResult
	account      ChannelAccountResult
	auditActions []string
}

func (f *channelStoreFake) UpdateMasterProfile(_ context.Context, workspaceID, rawPhone string) (MasterProfileResult, error) {
	f.profile = MasterProfileResult{WorkspaceID: workspaceID, MasterPhoneRaw: rawPhone, MasterPhoneNormalized: "+79990000000"}
	return f.profile, nil
}

func (f *channelStoreFake) SaveChannelSettings(_ context.Context, input ChannelAccountInput) (ChannelAccountResult, error) {
	f.account = ChannelAccountResult{ID: "cha_1", WorkspaceID: input.WorkspaceID, Provider: input.Provider, ChannelKind: input.ChannelKind}
	return f.account, nil
}

func (f *channelStoreFake) AddAuditLog(_ context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error {
	f.auditActions = append(f.auditActions, action)
	return nil
}

func TestChannelServiceRequiresUserActor(t *testing.T) {
	store := &channelStoreFake{}
	service := NewChannelService(store, DefaultPolicy{})
	operator := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: "ws_1", UserID: "user_1"}

	if _, err := service.UpdateMasterProfile(context.Background(), operator, "ws_1", "+79990000000"); err == nil {
		t.Fatalf("expected access denied for operator bot")
	}
}

func TestChannelServiceSaveChannelWritesAudit(t *testing.T) {
	store := &channelStoreFake{}
	service := NewChannelService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorUser, WorkspaceID: "ws_1", UserID: "user_1", Role: string(domain.RoleAdmin)}

	result, err := service.SaveChannelSettings(context.Background(), actor, ChannelAccountInput{
		WorkspaceID: "ws_1",
		Provider:    "telegram",
		ChannelKind: "telegram_client",
	})
	if err != nil {
		t.Fatalf("save channel: %v", err)
	}
	if result.ID != "cha_1" {
		t.Fatalf("unexpected channel result: %#v", result)
	}
	if len(store.auditActions) != 1 || store.auditActions[0] != "settings.channel_updated" {
		t.Fatalf("unexpected audit actions: %#v", store.auditActions)
	}
}
