package usecase

import (
	"context"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
)

type BotSessionInput struct {
	WorkspaceID string
	BotKind     string
	Scope       string
	ActorType   string
	ActorID     string
	State       string
	ExpiresAt   time.Time
}

type BotSessionResult struct {
	ID          string
	WorkspaceID string
	BotKind     string
	Scope       string
	ActorType   string
	ActorID     string
	State       string
	Payload     string
	ExpiresAt   time.Time
	UpdatedAt   time.Time
}

type ClientBotRouteInput struct {
	ChannelAccountID              string
	ExternalChatID                string
	SelectedWorkspaceID           string
	SelectedMasterPhoneNormalized string
	State                         string
	ExpiresAt                     time.Time
}

type ClientBotRouteResult struct {
	ChannelAccountID              string
	ExternalChatID                string
	SelectedWorkspaceID           string
	SelectedMasterPhoneNormalized string
	State                         string
	ExpiresAt                     time.Time
	UpdatedAt                     time.Time
}

type BotSessionStore interface {
	SaveBotSession(ctx context.Context, input BotSessionInput, payload any) (BotSessionResult, error)
	DeleteBotSession(ctx context.Context, workspaceID, scope, actorType, actorID string) error
	SaveClientBotRoute(ctx context.Context, input ClientBotRouteInput) (ClientBotRouteResult, error)
	ClearClientBotRoute(ctx context.Context, channelAccountID, externalChatID string) error
}

type BotSessionService struct {
	store  BotSessionStore
	policy Policy
}

func NewBotSessionService(store BotSessionStore, policy Policy) BotSessionService {
	return BotSessionService{store: store, policy: policy}
}

func (s BotSessionService) StartSession(ctx context.Context, actor domain.Actor, input BotSessionInput, payload any) (BotSessionResult, error) {
	if err := s.policy.CanManageBotSession(actor, input.WorkspaceID); err != nil {
		return BotSessionResult{}, err
	}
	return s.store.SaveBotSession(ctx, input, payload)
}

func (s BotSessionService) ClearSession(ctx context.Context, actor domain.Actor, workspaceID, scope, actorType, actorID string) error {
	if err := s.policy.CanManageBotSession(actor, workspaceID); err != nil {
		return err
	}
	return s.store.DeleteBotSession(ctx, workspaceID, scope, actorType, actorID)
}

func (s BotSessionService) StoreClientRoute(ctx context.Context, actor domain.Actor, input ClientBotRouteInput) (ClientBotRouteResult, error) {
	if err := s.policy.CanManageBotSession(actor, actor.WorkspaceID); err != nil {
		return ClientBotRouteResult{}, err
	}
	return s.store.SaveClientBotRoute(ctx, input)
}

func (s BotSessionService) ClearClientRoute(ctx context.Context, actor domain.Actor, channelAccountID, externalChatID string) error {
	if err := s.policy.CanManageBotSession(actor, actor.WorkspaceID); err != nil {
		return err
	}
	return s.store.ClearClientBotRoute(ctx, channelAccountID, externalChatID)
}
