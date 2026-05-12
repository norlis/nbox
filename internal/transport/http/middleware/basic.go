package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"nbox/internal/application"
	auth "nbox/internal/auth"
)

func (a *Authn) tryBasicAuth(r *http.Request) (context.Context, error) {
	username, pass, ok := r.BasicAuth()
	if !ok {
		return nil, ErrInvalidAuthHeaderFormat
	}

	user, err := a.repository.ValidatePassword(r.Context(), username, pass)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) || errors.Is(err, auth.ErrInvalidPassword) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to validate password: %w", err)
	}

	return application.NewContextWithUser(r.Context(), application.User{
		Name:  user.Username,
		Roles: user.Roles,
	}), nil
}
