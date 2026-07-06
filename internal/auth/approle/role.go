// Package approle implements the AppRole authentication method for nbox
// M2M auth. Service accounts present a (role_id, secret_id) pair to
// exchange for a nbox JWT. Multiple active secret_id hashes per role
// enable zero-downtime rotation.
//
// Bootstrap (admin):
//
//	nbox-cli config approle generate --name watcher-agent --roles entrypushd
//
// Authentication (agent → entrypushd gRPC):
//
//	metadata: authorization: AppRole <base64(JSON{role_id, secret_id})>
//
// The interceptor in internal/entrypushd/grpc validates the credential
// via approle.Authenticator and mints an internal JWT (aud=[nbox,
// entrypushd], TTL=15min) used for downstream HTTP calls to nbox.
// The agent never sees the JWT.
//
// Wire format for the credential is consumed by Authenticator and
// produced by client.BuildCredential.
package approle

import "time"

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// Role represents a service account in AppRole semantics. One role can
// have multiple active SecretHashes to support zero-downtime rotation.
type Role struct {
	// ID is the UUID assigned at creation time. Stable across rotations.
	ID string `json:"id"`

	// Name is the human-readable identifier (e.g., "entrypushd-service").
	// Appears in logs and JWT claims for traceability.
	Name string `json:"name"`

	// Roles are the OPA role names this principal gets when authenticated.
	Roles []string `json:"roles"`

	// Attributes are optional key-value pairs that flow into the issued
	// JWT as custom claims (under `attributes`). Useful for future
	// ABAC-style policies that need fine-grained scoping beyond roles.
	// OPA does not consume these — they're seeds for ABAC migration.
	//
	// The minter strips only one reserved key, "name", which becomes the
	// JWT Name claim. Every other entry flows verbatim to the `attributes`
	// claim. The JWT Audience is hardcoded to ["nbox","entrypushd"] and is
	// NOT configurable per-role; setting Attributes["aud"] has no effect
	// other than appearing as an opaque attribute in the token.
	//
	// Example:
	//   {"allowed_prefixes": "passbox/banking/", "env": "production"}
	Attributes map[string]string `json:"attributes,omitempty"`

	// AllowedCIDRs optionally restricts which source IPs may present
	// this role's credentials. Empty = no restriction. Non-empty =
	// login rejected unless source IP matches at least one CIDR.
	//
	// Example: ["10.0.0.0/8", "172.16.0.0/12"]
	AllowedCIDRs []string `json:"allowed_cidrs,omitempty"`

	// SecretHashes are bcrypt hashes of currently-valid secret_ids.
	// Any one of them matching a presented secret_id authenticates the
	// role. Rotate by adding a new hash, distributing the secret_id to
	// clients, then removing the old hash after grace period.
	SecretHashes []SecretHash `json:"secret_hashes"`

	// Status is "active" (default) or "disabled" (rejects all logins).
	Status Status `json:"status"`
}

type SecretHash struct {
	// Hash is the bcrypt hash of the secret_id (cost 10 minimum).
	Hash string `json:"hash"`

	// CreatedAt is informational — for ops to know which hash is older.
	CreatedAt time.Time `json:"created_at"`

	// ExpiresAt, when non-nil, defines when this hash stops being valid.
	// After expiration, Authenticate skips this hash. nil = no expiry.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
