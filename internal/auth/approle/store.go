package approle

import (
	"context"
	"encoding/json"
	"fmt"
)

// Store resolves AppRole credentials by role ID. Implementations must
// be safe for concurrent use.
type Store interface {
	GetRole(ctx context.Context, roleID string) (*Role, error)
}

type InMemory struct {
	roles map[string]Role
}

// NewInMemory builds a Store from a JSON array of Role objects.
// Empty input (nil or zero-length) produces an empty store with no roles.
// If the same role ID appears more than once, the last occurrence wins.
//
// Example payload:
//
//	[
//	  {
//	    "id": "ce3f9e0a-...",
//	    "name": "entrypushd-service",
//	    "roles": ["entrypushd"],
//	    "secret_hashes": [
//	      {"hash": "$2a$10$...", "created_at": "2026-05-19T00:00:00Z"}
//	    ],
//	    "status": "active"
//	  }
//	]
func NewInMemory(rawJSON []byte) (*InMemory, error) {
	if len(rawJSON) == 0 {
		return &InMemory{roles: map[string]Role{}}, nil
	}

	var roles []Role
	if err := json.Unmarshal(rawJSON, &roles); err != nil {
		return nil, fmt.Errorf("approle: parse roles json: %w", err)
	}

	s := &InMemory{roles: make(map[string]Role, len(roles))}
	for _, r := range roles {
		if r.ID == "" {
			return nil, fmt.Errorf("approle: role with empty id (name=%q)", r.Name)
		}
		if r.Status == "" {
			r.Status = StatusActive
		}
		s.roles[r.ID] = r
	}
	return s, nil
}

func (s *InMemory) GetRole(_ context.Context, roleID string) (*Role, error) {
	r, ok := s.roles[roleID]
	if !ok {
		return nil, ErrRoleNotFound
	}
	return &r, nil
}
