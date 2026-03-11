package usecase

import "github.com/vital/rendycrm-app/internal/domain"

type Policy interface {
	CanReplyDialog(actor domain.Actor, workspaceID string) error
	CanManageDialog(actor domain.Actor, workspaceID string) error
	CanManageBooking(actor domain.Actor, workspaceID string) error
	CanManageCustomer(actor domain.Actor, workspaceID string) error
	CanManageReviews(actor domain.Actor, workspaceID string) error
	CanManageChannels(actor domain.Actor, workspaceID string) error
	CanManageOperatorLink(actor domain.Actor, workspaceID string) error
	CanManageBotSession(actor domain.Actor, workspaceID string) error
	CanChangeSettings(actor domain.Actor, workspaceID string) error
}

type DefaultPolicy struct{}

func (DefaultPolicy) CanReplyDialog(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser, domain.ActorOperatorBot:
		return nil
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanManageDialog(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser, domain.ActorOperatorBot:
		return nil
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanManageBooking(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser, domain.ActorOperatorBot, domain.ActorCustomerBot:
		return nil
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanManageCustomer(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser, domain.ActorOperatorBot:
		return nil
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanManageReviews(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser, domain.ActorOperatorBot:
		return nil
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanManageChannels(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser:
		if isAdminUser(actor) {
			return nil
		}
		return domain.ErrAccessDenied
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanManageOperatorLink(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser:
		if isAdminUser(actor) {
			return nil
		}
		return domain.ErrAccessDenied
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanManageBotSession(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser, domain.ActorOperatorBot, domain.ActorCustomerBot, domain.ActorSystem:
		return nil
	default:
		return domain.ErrAccessDenied
	}
}

func (DefaultPolicy) CanChangeSettings(actor domain.Actor, workspaceID string) error {
	if err := requireWorkspace(actor, workspaceID); err != nil {
		return err
	}
	switch actor.Kind {
	case domain.ActorUser:
		if isAdminUser(actor) {
			return nil
		}
		return domain.ErrAccessDenied
	case domain.ActorOperatorBot:
		return nil
	default:
		return domain.ErrAccessDenied
	}
}

func requireWorkspace(actor domain.Actor, workspaceID string) error {
	if actor.Kind == domain.ActorSystem {
		return nil
	}
	if actor.WorkspaceID == "" || actor.WorkspaceID != workspaceID {
		return domain.ErrAccessDenied
	}
	return nil
}

func isAdminUser(actor domain.Actor) bool {
	return actor.Kind == domain.ActorUser && actor.Role == string(domain.RoleAdmin)
}
