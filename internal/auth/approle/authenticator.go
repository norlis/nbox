package approle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"time"

	"golang.org/x/crypto/bcrypt"
	"nbox/internal/auth"
)

// MethodName is the unique identifier for this authentication method.
// It matches what (*Authenticator).Method() returns and what the
// registry uses to dispatch credentials with the "AppRole" scheme.
const MethodName = "approle"

// Authenticator implements auth.Authenticator for the AppRole method.
type Authenticator struct {
	store Store
	now   func() time.Time // injectable for tests
}

// New builds an authenticator backed by store. Use NewWithClock for tests.
func New(store Store) *Authenticator {
	return &Authenticator{store: store, now: time.Now}
}

// NewWithClock is like New but lets tests inject a deterministic clock.
func NewWithClock(store Store, clock func() time.Time) *Authenticator {
	return &Authenticator{store: store, now: clock}
}

func (a *Authenticator) Method() string { return MethodName }

type credential struct {
	RoleID   string `json:"role_id"`
	SecretID string `json:"secret_id"`
}

// Authenticate verifies (role_id, secret_id) against the configured Store.
// Returns one of: nil error + valid Identity; ErrInvalidCredential;
// ErrRoleNotFound; ErrRoleDisabled; ErrInvalidSecret; ErrSourceNotAllowed;
// or a wrapped parse/store error.
func (a *Authenticator) Authenticate(ctx context.Context, raw []byte) (*auth.Identity, error) {
	var c credential
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("approle: invalid credential format: %w", err)
	}
	if c.RoleID == "" || c.SecretID == "" {
		return nil, ErrInvalidCredential
	}

	role, err := a.store.GetRole(ctx, c.RoleID)
	if err != nil {
		if errors.Is(err, ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("approle: role lookup: %w", err)
	}

	if role.Status != StatusActive {
		return nil, ErrRoleDisabled
	}

	// CIDR enforcement: if AllowedCIDRs is set, reject logins from IPs
	// outside the list. Defense in depth against leaked secret_ids.
	if len(role.AllowedCIDRs) > 0 {
		sourceIP, _ := auth.SourceIPFromContext(ctx)
		if !matchesAnyCIDR(sourceIP, role.AllowedCIDRs) {
			return nil, ErrSourceNotAllowed
		}
	}

	if !a.matchesAnyHash(role.SecretHashes, c.SecretID) {
		return nil, ErrInvalidSecret
	}

	// Merge role.Attributes into Meta. "name" is the reserved key for
	// the JWT Name claim; it overrides any attribute with the same key.
	meta := make(map[string]string, len(role.Attributes)+1)
	maps.Copy(meta, role.Attributes)
	meta["name"] = role.Name

	return &auth.Identity{
		Method:  MethodName,
		Subject: role.ID,
		Roles:   role.Roles,
		Meta:    meta,
	}, nil
}

// matchesAnyCIDR returns true if sourceIP falls within any of the CIDRs.
// Invalid CIDR strings in the list are silently skipped (the Store should
// validate at parse time). Empty sourceIP never matches.
func matchesAnyCIDR(sourceIP string, cidrs []string) bool {
	if sourceIP == "" {
		return false
	}
	ip := net.ParseIP(sourceIP)
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		_, network, err := net.ParseCIDR(c)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// matchesAnyHash iterates ALL non-expired hashes and accumulates a match
// result without early-returning on the first match. This is intentional:
// bcrypt is slow by design, so returning as soon as one hash matches leaks
// timing information about which position matched (timing-oracle). By always
// evaluating every non-expired hash we eliminate that side-channel.
func (a *Authenticator) matchesAnyHash(hashes []SecretHash, secretID string) bool {
	now := a.now()
	matched := false
	for _, sh := range hashes {
		if sh.ExpiresAt != nil && !sh.ExpiresAt.After(now) {
			continue // expired hashes are skipped (cheap check, no oracle on secret validity)
		}
		if bcrypt.CompareHashAndPassword([]byte(sh.Hash), []byte(secretID)) == nil {
			matched = true // keep iterating: do not leak which/position via timing
		}
	}
	return matched
}
