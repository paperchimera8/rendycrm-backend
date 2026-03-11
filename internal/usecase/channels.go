package usecase

import (
	"context"
	"strings"

	"github.com/vital/rendycrm-app/internal/domain"
)

type MasterProfileResult struct {
	WorkspaceID           string
	MasterPhoneRaw        string
	MasterPhoneNormalized string
	TelegramEnabled       bool
}

type ChannelAccountInput struct {
	WorkspaceID     string
	Provider        string
	ChannelKind     string
	AccountScope    string
	Name            string
	Connected       bool
	IsEnabled       bool
	BotUsername     string
	WebhookSecret   string
	EncryptedToken  string
	TokenConfigured bool
}

type ChannelAccountResult struct {
	ID              string
	WorkspaceID     string
	Provider        string
	ChannelKind     string
	AccountScope    string
	Name            string
	Connected       bool
	IsEnabled       bool
	BotUsername     string
	TokenConfigured bool
}

type ChannelStore interface {
	UpdateMasterProfile(ctx context.Context, workspaceID, rawPhone string) (MasterProfileResult, error)
	SaveChannelSettings(ctx context.Context, input ChannelAccountInput) (ChannelAccountResult, error)
	AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error
}

type ChannelService struct {
	store  ChannelStore
	policy Policy
}

func NewChannelService(store ChannelStore, policy Policy) ChannelService {
	return ChannelService{store: store, policy: policy}
}

func (s ChannelService) UpdateMasterProfile(ctx context.Context, actor domain.Actor, workspaceID, rawPhone string) (MasterProfileResult, error) {
	if err := s.policy.CanManageChannels(actor, workspaceID); err != nil {
		return MasterProfileResult{}, err
	}
	result, err := s.store.UpdateMasterProfile(ctx, workspaceID, strings.TrimSpace(rawPhone))
	if err != nil {
		return MasterProfileResult{}, err
	}
	_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "settings.master_profile_updated", "workspace", workspaceID, nil)
	return result, nil
}

func (s ChannelService) SaveChannelSettings(ctx context.Context, actor domain.Actor, input ChannelAccountInput) (ChannelAccountResult, error) {
	if err := s.policy.CanManageChannels(actor, input.WorkspaceID); err != nil {
		return ChannelAccountResult{}, err
	}
	result, err := s.store.SaveChannelSettings(ctx, input)
	if err != nil {
		return ChannelAccountResult{}, err
	}
	_ = s.store.AddAuditLog(ctx, input.WorkspaceID, actor.UserID, "settings.channel_updated", "channel_account", result.ID, map[string]any{"provider": input.Provider, "kind": input.ChannelKind})
	return result, nil
}
