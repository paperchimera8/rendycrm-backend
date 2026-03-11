package usecase

import (
	"context"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
)

type OperatorLinkResult struct {
	UserID         string
	WorkspaceID    string
	TelegramUserID string
	TelegramChatID string
}

type OperatorLinkStore interface {
	LinkOperatorTelegram(ctx context.Context, code, telegramUserID, telegramChatID string) (OperatorLinkResult, error)
	CreateOperatorLinkCode(ctx context.Context, workspaceID, userID, botUsername string) (OperatorLinkCodeResult, error)
	UnlinkOperatorTelegram(ctx context.Context, workspaceID, userID string) error
	AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error
}

type OperatorLinkCodeResult struct {
	ID          string
	UserID      string
	WorkspaceID string
	Code        string
	ExpiresAt   time.Time
	DeepLink    string
}

type OperatorLinkService struct {
	store  OperatorLinkStore
	policy Policy
}

func NewOperatorLinkService(store OperatorLinkStore, policy Policy) OperatorLinkService {
	return OperatorLinkService{store: store, policy: policy}
}

func (s OperatorLinkService) LinkTelegram(ctx context.Context, code, telegramUserID, telegramChatID string) (OperatorLinkResult, error) {
	result, err := s.store.LinkOperatorTelegram(ctx, code, telegramUserID, telegramChatID)
	if err != nil {
		return OperatorLinkResult{}, err
	}
	_ = s.store.AddAuditLog(ctx, result.WorkspaceID, result.UserID, "operator_bot.link", "operator_bot_binding", result.UserID, map[string]string{"telegramChatID": result.TelegramChatID})
	return result, nil
}

func (s OperatorLinkService) CreateLinkCode(ctx context.Context, actor domain.Actor, workspaceID, userID, botUsername string) (OperatorLinkCodeResult, error) {
	if err := s.policy.CanManageOperatorLink(actor, workspaceID); err != nil {
		return OperatorLinkCodeResult{}, err
	}
	result, err := s.store.CreateOperatorLinkCode(ctx, workspaceID, userID, botUsername)
	if err != nil {
		return OperatorLinkCodeResult{}, err
	}
	_ = s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "operator_bot.link_code_created", "operator_bot_link_code", userID, nil)
	return result, nil
}

func (s OperatorLinkService) UnlinkTelegram(ctx context.Context, actor domain.Actor, workspaceID, userID string) error {
	if err := s.policy.CanManageOperatorLink(actor, workspaceID); err != nil {
		return err
	}
	if err := s.store.UnlinkOperatorTelegram(ctx, workspaceID, userID); err != nil {
		return err
	}
	return s.store.AddAuditLog(ctx, workspaceID, actor.UserID, "operator_bot.unlink", "operator_bot_binding", userID, nil)
}
