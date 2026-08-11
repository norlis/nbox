package application

import "context"

type User struct {
	Name  string   `json:"username"`
	Roles []string `json:"roles"`
}

type userSessionKey struct{}

func NewContextWithUser(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, userSessionKey{}, user)
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userSessionKey{}).(User)
	return u, ok
}

// DefaultActor is the username recorded when no authenticated user is in context.
const DefaultActor = "ghost"

// ActorFromContext returns the authenticated user's name, or DefaultActor when
// no user is present in ctx.
func ActorFromContext(ctx context.Context) string {
	if u, ok := UserFromContext(ctx); ok {
		return u.Name
	}
	return DefaultActor
}
