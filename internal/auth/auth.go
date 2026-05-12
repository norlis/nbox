package auth

import "context"

type User struct {
	Username string
	Password string
	Roles    []string
	Status   string
}

type Store interface {
	FindByUsername(ctx context.Context, username string) (*User, error)
	ValidatePassword(ctx context.Context, username, password string) (*User, error)
}
