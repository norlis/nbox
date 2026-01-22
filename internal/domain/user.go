package domain

import (
	"context"
)

//
// var (
//	ErrUserNotFound    = errors.New("user not found")
//	ErrInvalidPassword = errors.New("invalid password") // ✅ Nuevo error específico
//)

type User struct {
	Username string
	Password string
	Roles    []string
	Status   string
}

type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (*User, error)
	ValidatePassword(ctx context.Context, username, password string) (*User, error)
}
