package ability

import (
	"context"
	"errors"

	"github.com/fibegg/go-fibe/internal/models"
)

type Action string
type Resource string

const (
	ActionRead   Action = "read"
	ActionManage Action = "manage"
	ActionRun    Action = "run"

	ResourceMonitor     Resource = "monitor"
	ResourceIncident    Resource = "incident"
	ResourceJob         Resource = "job"
	ResourceMaintenance Resource = "maintenance"
)

type contextKey struct{}

var (
	ErrAuthenticationRequired = errors.New("authentication required")
	ErrNotAuthorized          = errors.New("not authorized")
)

func WithUser(ctx context.Context, user models.CurrentUser) context.Context {
	return context.WithValue(ctx, contextKey{}, user)
}

func UserFromContext(ctx context.Context) (models.CurrentUser, bool) {
	user, ok := ctx.Value(contextKey{}).(models.CurrentUser)
	return user, ok
}

func Require(ctx context.Context, action Action, resource Resource) (models.CurrentUser, error) {
	user, ok := UserFromContext(ctx)
	if !ok {
		return models.CurrentUser{}, ErrAuthenticationRequired
	}
	if Can(user.Role, action, resource) {
		return user, nil
	}
	return models.CurrentUser{}, ErrNotAuthorized
}

func Can(role string, action Action, resource Resource) bool {
	switch role {
	case "admin":
		return true
	case "operator":
		if resource == ResourceMaintenance && action != ActionRead {
			return false
		}
		return action == ActionRead || action == ActionManage || action == ActionRun
	case "viewer":
		return action == ActionRead
	default:
		return false
	}
}
