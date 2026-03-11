package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/vital/rendycrm-app/internal/domain"
)

type botSessionStoreFake struct {
	sessionCleared bool
	routeCleared   bool
	sessionSaved   bool
	routeSaved     bool
}

func (f *botSessionStoreFake) SaveBotSession(_ context.Context, input BotSessionInput, payload any) (BotSessionResult, error) {
	f.sessionSaved = true
	return BotSessionResult{WorkspaceID: input.WorkspaceID, ActorID: input.ActorID, State: input.State}, nil
}

func (f *botSessionStoreFake) DeleteBotSession(_ context.Context, workspaceID, scope, actorType, actorID string) error {
	f.sessionCleared = true
	return nil
}

func (f *botSessionStoreFake) SaveClientBotRoute(_ context.Context, input ClientBotRouteInput) (ClientBotRouteResult, error) {
	f.routeSaved = true
	return ClientBotRouteResult{ChannelAccountID: input.ChannelAccountID, ExternalChatID: input.ExternalChatID, State: input.State}, nil
}

func (f *botSessionStoreFake) ClearClientBotRoute(_ context.Context, channelAccountID, externalChatID string) error {
	f.routeCleared = true
	return nil
}

func TestBotSessionServiceAllowsSystemRouteStorage(t *testing.T) {
	store := &botSessionStoreFake{}
	service := NewBotSessionService(store, DefaultPolicy{})

	_, err := service.StoreClientRoute(context.Background(), domain.SystemActor(), ClientBotRouteInput{
		ChannelAccountID: "cha_1",
		ExternalChatID:   "chat_1",
		State:            "awaiting_master_phone",
		ExpiresAt:        time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("store client route: %v", err)
	}
	if !store.routeSaved {
		t.Fatalf("expected route to be saved")
	}
}

func TestBotSessionServiceDeniesWorkspaceMismatch(t *testing.T) {
	store := &botSessionStoreFake{}
	service := NewBotSessionService(store, DefaultPolicy{})
	actor := domain.Actor{Kind: domain.ActorOperatorBot, WorkspaceID: "ws_1", UserID: "user_1"}

	if _, err := service.StartSession(context.Background(), actor, BotSessionInput{
		WorkspaceID: "ws_2",
		BotKind:     "telegram_operator",
		Scope:       "operator",
		ActorType:   "user",
		ActorID:     "user_1",
		State:       "awaiting_reply",
	}, nil); err == nil {
		t.Fatalf("expected access denied")
	}
}
