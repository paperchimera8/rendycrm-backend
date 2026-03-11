package usecase

import (
	"context"

	"github.com/vital/rendycrm-app/internal/domain"
)

type BotConfigState struct {
	AutoReply      bool
	HandoffEnabled bool
	Tone           string
	WelcomeMessage string
	HandoffMessage string
}

type FAQItem struct {
	ID       string
	Question string
	Answer   string
}

type BotSettingsStore interface {
	LoadBotConfig(ctx context.Context, workspaceID string) (BotConfigState, []FAQItem, error)
	SaveBotConfig(ctx context.Context, workspaceID string, config BotConfigState, faq []FAQItem) error
	AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error
}

type BotSettingsService struct {
	store  BotSettingsStore
	policy Policy
}

func NewBotSettingsService(store BotSettingsStore, policy Policy) BotSettingsService {
	return BotSettingsService{store: store, policy: policy}
}

func (s BotSettingsService) ToggleAutoReply(ctx context.Context, actor domain.Actor, workspaceID string, enabled bool) error {
	if err := s.policy.CanChangeSettings(actor, workspaceID); err != nil {
		return err
	}
	config, faq, err := s.store.LoadBotConfig(ctx, workspaceID)
	if err != nil {
		return err
	}
	config.AutoReply = enabled
	if err := s.store.SaveBotConfig(ctx, workspaceID, config, faq); err != nil {
		return err
	}
	action := "settings.auto_reply_disabled"
	if enabled {
		action = "settings.auto_reply_enabled"
	}
	return s.store.AddAuditLog(ctx, workspaceID, actor.UserID, action, "bot_config", workspaceID, nil)
}

func (s BotSettingsService) UpdateConfig(ctx context.Context, actor domain.Actor, workspaceID string, config BotConfigState, faq []FAQItem) (BotConfigState, []FAQItem, error) {
	if err := s.policy.CanChangeSettings(actor, workspaceID); err != nil {
		return BotConfigState{}, nil, err
	}
	if err := s.store.SaveBotConfig(ctx, workspaceID, config, faq); err != nil {
		return BotConfigState{}, nil, err
	}
	updated, updatedFAQ, err := s.store.LoadBotConfig(ctx, workspaceID)
	if err != nil {
		return BotConfigState{}, nil, err
	}
	_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "settings.bot_config_updated", "bot_config", workspaceID, nil)
	return updated, updatedFAQ, nil
}
