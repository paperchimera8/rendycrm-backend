package domain

import "errors"

type ActorKind string

const (
	ActorUser        ActorKind = "user"
	ActorOperatorBot ActorKind = "operator_bot"
	ActorCustomerBot ActorKind = "customer_bot"
	ActorSystem      ActorKind = "system"
)

var ErrAccessDenied = errors.New("access denied")

type Actor struct {
	Kind        ActorKind
	WorkspaceID string
	UserID      string
	Role        string
}

func SystemActor() Actor {
	return Actor{Kind: ActorSystem}
}
