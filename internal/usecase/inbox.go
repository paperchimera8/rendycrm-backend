package usecase

import (
	"context"
	"time"
)

type InboundProfile struct {
	Name     string
	Username string
	Phone    string
}

type InboundInput struct {
	Provider          string
	ChannelAccountID  string
	ExternalChatID    string
	ExternalMessageID string
	Text              string
	Timestamp         time.Time
	Profile           InboundProfile
}

type InboundResult struct {
	WorkspaceID     string
	ConversationID  string
	MessageID       string
	CustomerID      string
	ConversationNew bool
	Stored          bool
}

type InboxStore interface {
	ReceiveInboundMessage(ctx context.Context, input InboundInput) (InboundResult, error)
	ReceiveInboundMessageForWorkspace(ctx context.Context, workspaceID string, input InboundInput) (InboundResult, error)
}

type InboxService struct {
	store InboxStore
}

func NewInboxService(store InboxStore) InboxService {
	return InboxService{store: store}
}

func (s InboxService) ReceiveInboundMessage(ctx context.Context, input InboundInput) (InboundResult, error) {
	return s.store.ReceiveInboundMessage(ctx, input)
}

func (s InboxService) ReceiveInboundMessageForWorkspace(ctx context.Context, workspaceID string, input InboundInput) (InboundResult, error) {
	return s.store.ReceiveInboundMessageForWorkspace(ctx, workspaceID, input)
}
