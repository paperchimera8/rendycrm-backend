package usecase

import (
	"context"
	"strings"

	"github.com/vital/rendycrm-app/internal/domain"
)

type DialogStore interface {
	ReplyToDialog(ctx context.Context, workspaceID, conversationID, userID, text string) (DialogReplyResult, error)
	AssignDialog(ctx context.Context, workspaceID, conversationID, userID string) error
	SetDialogAutomation(ctx context.Context, workspaceID, conversationID, status, intent string) error
	SetDialogStatus(ctx context.Context, workspaceID, conversationID, status string) error
	AddAuditLog(ctx context.Context, workspaceID, userID, action, entityType, entityID string, payload any) error
}

type DialogReplyResult struct {
	ConversationID string
	MessageID      string
}

type DialogService struct {
	store  DialogStore
	policy Policy
}

func NewDialogService(store DialogStore, policy Policy) DialogService {
	return DialogService{store: store, policy: policy}
}

func (s DialogService) ReplyToDialog(ctx context.Context, actor domain.Actor, conversationID, text string) (DialogReplyResult, error) {
	if err := s.policy.CanReplyDialog(actor, actor.WorkspaceID); err != nil {
		return DialogReplyResult{}, err
	}
	return s.store.ReplyToDialog(ctx, actor.WorkspaceID, conversationID, actor.UserID, strings.TrimSpace(text))
}

func (s DialogService) TakeDialogByHuman(ctx context.Context, actor domain.Actor, conversationID string) error {
	if err := s.policy.CanManageDialog(actor, actor.WorkspaceID); err != nil {
		return err
	}
	if err := s.store.AssignDialog(ctx, actor.WorkspaceID, conversationID, actor.UserID); err != nil {
		return err
	}
	if err := s.store.SetDialogAutomation(ctx, actor.WorkspaceID, conversationID, "human", "human_request"); err != nil {
		return err
	}
	return s.store.AddAuditLog(ctx, actor.WorkspaceID, actor.UserID, "dialog.take_human", "conversation", conversationID, nil)
}

func (s DialogService) ReturnDialogToAuto(ctx context.Context, actor domain.Actor, conversationID string) error {
	if err := s.policy.CanManageDialog(actor, actor.WorkspaceID); err != nil {
		return err
	}
	if err := s.store.SetDialogAutomation(ctx, actor.WorkspaceID, conversationID, "auto", "other"); err != nil {
		return err
	}
	return s.store.AddAuditLog(ctx, actor.WorkspaceID, actor.UserID, "dialog.return_to_auto", "conversation", conversationID, nil)
}

func (s DialogService) CloseDialog(ctx context.Context, actor domain.Actor, conversationID string) error {
	if err := s.policy.CanManageDialog(actor, actor.WorkspaceID); err != nil {
		return err
	}
	if err := s.store.SetDialogStatus(ctx, actor.WorkspaceID, conversationID, "closed"); err != nil {
		return err
	}
	return s.store.AddAuditLog(ctx, actor.WorkspaceID, actor.UserID, "dialog.closed", "conversation", conversationID, nil)
}

func (s DialogService) ReopenDialog(ctx context.Context, actor domain.Actor, conversationID string) error {
	if err := s.policy.CanManageDialog(actor, actor.WorkspaceID); err != nil {
		return err
	}
	if err := s.store.SetDialogStatus(ctx, actor.WorkspaceID, conversationID, "human"); err != nil {
		return err
	}
	return s.store.AddAuditLog(ctx, actor.WorkspaceID, actor.UserID, "dialog.reopened", "conversation", conversationID, nil)
}
