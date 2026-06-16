package store

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"nbox/internal/auth"
)

// dummyHash is a precomputed bcrypt hash used when a username is not found, so
// that the not-found path performs ~the same amount of work as the wrong-password
// path. This prevents username enumeration via response-time differences.
//
//nolint:gochecknoglobals
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("nbox-dummy-password"), bcrypt.DefaultCost)

type userSchema map[string]struct {
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
	Status   string   `json:"status"`
}

type users map[string]auth.User

// InMemory es un Store de usuarios respaldado por un mapa en memoria
// inicializado a partir de credenciales JSON.
// The users map is immutable after construction; no mutex is needed.
type InMemory struct {
	users users
}

// NewInMemory crea un Store en memoria a partir de credenciales en JSON.
func NewInMemory(jsonCredentials []byte) (auth.Store, error) {
	var schema userSchema
	if err := json.Unmarshal(jsonCredentials, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	store := &InMemory{users: make(users, len(schema))}

	for username, data := range schema {
		store.users[username] = auth.User{
			Username: username,
			Password: data.Password,
			Roles:    data.Roles,
			Status:   data.Status,
		}
	}

	return store, nil
}

func (r *InMemory) FindByUsername(_ context.Context, username string) (*auth.User, error) {
	user, exists := r.users[username]
	if !exists {
		return nil, auth.ErrUserNotFound
	}
	return &user, nil
}

func (r *InMemory) ValidatePassword(ctx context.Context, username, password string) (*auth.User, error) {
	user, err := r.FindByUsername(ctx, username)
	if err != nil {
		// Run a bcrypt comparison against the dummy hash so that the not-found
		// path takes approximately the same time as the wrong-password path,
		// preventing username enumeration via timing side-channel.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, auth.ErrInvalidPassword
	}

	return user, nil
}
