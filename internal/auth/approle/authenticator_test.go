package approle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"nbox/internal/auth"
	"nbox/internal/auth/approle"
)

// stubStore lets tests inject a single role.
type stubStore struct {
	role *approle.Role
	err  error
}

func (s *stubStore) GetRole(_ context.Context, _ string) (*approle.Role, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.role, nil
}

func bcryptHash(t *testing.T, plaintext string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

func validRole(t *testing.T, secretID string) *approle.Role {
	t.Helper()
	return &approle.Role{
		ID:    "role-uuid",
		Name:  "test-role",
		Roles: []string{"viewer"},
		SecretHashes: []approle.SecretHash{
			{Hash: bcryptHash(t, secretID), CreatedAt: time.Now()},
		},
		Status: approle.StatusActive,
	}
}

func TestAuthenticate_ValidCredentials_ReturnsIdentity(t *testing.T) {
	store := &stubStore{role: validRole(t, "secret-abc")}
	a := approle.New(store)

	cred := []byte(`{"role_id":"role-uuid","secret_id":"secret-abc"}`)
	id, err := a.Authenticate(context.Background(), cred)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Method != "approle" {
		t.Errorf("Method = %q", id.Method)
	}
	if id.Subject != "role-uuid" {
		t.Errorf("Subject = %q", id.Subject)
	}
	if !slices.Equal(id.Roles, []string{"viewer"}) {
		t.Errorf("Roles = %v", id.Roles)
	}
	if id.Meta["name"] != "test-role" {
		t.Errorf("Meta[name] = %q", id.Meta["name"])
	}
}

func TestAuthenticate_InvalidSecret_ReturnsErrInvalidSecret(t *testing.T) {
	store := &stubStore{role: validRole(t, "secret-abc")}
	a := approle.New(store)

	cred := []byte(`{"role_id":"role-uuid","secret_id":"wrong-secret"}`)
	if _, err := a.Authenticate(context.Background(), cred); !errors.Is(err, approle.ErrInvalidSecret) {
		t.Fatalf("want ErrInvalidSecret, got %v", err)
	}
}

func TestAuthenticate_UnknownRole_ReturnsErrRoleNotFound(t *testing.T) {
	store := &stubStore{err: approle.ErrRoleNotFound}
	a := approle.New(store)

	cred := []byte(`{"role_id":"missing","secret_id":"any"}`)
	if _, err := a.Authenticate(context.Background(), cred); !errors.Is(err, approle.ErrRoleNotFound) {
		t.Fatalf("want ErrRoleNotFound, got %v", err)
	}
}

func TestAuthenticate_DisabledRole_ReturnsErrRoleDisabled(t *testing.T) {
	role := validRole(t, "secret-abc")
	role.Status = approle.StatusDisabled
	a := approle.New(&stubStore{role: role})

	cred := []byte(`{"role_id":"role-uuid","secret_id":"secret-abc"}`)
	if _, err := a.Authenticate(context.Background(), cred); !errors.Is(err, approle.ErrRoleDisabled) {
		t.Fatalf("want ErrRoleDisabled, got %v", err)
	}
}

func TestAuthenticate_MalformedJSON_ReturnsParseError(t *testing.T) {
	a := approle.New(&stubStore{role: validRole(t, "x")})

	_, err := a.Authenticate(context.Background(), []byte(`not json`))
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "invalid credential format") {
		t.Errorf("err %q should mention 'invalid credential format'", err)
	}
}

func TestAuthenticate_MissingFields_ReturnsErrInvalidCredential(t *testing.T) {
	a := approle.New(&stubStore{role: validRole(t, "x")})

	if _, err := a.Authenticate(context.Background(), []byte(`{"role_id":"r"}`)); !errors.Is(err, approle.ErrInvalidCredential) {
		t.Fatalf("missing secret_id: want ErrInvalidCredential, got %v", err)
	}

	if _, err := a.Authenticate(context.Background(), []byte(`{"secret_id":"s"}`)); !errors.Is(err, approle.ErrInvalidCredential) {
		t.Fatalf("missing role_id: want ErrInvalidCredential, got %v", err)
	}
}

func TestAuthenticate_ExpiredHashSkipped_OtherMatches(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	role := &approle.Role{
		ID:    "role-uuid",
		Name:  "test-role",
		Roles: []string{"viewer"},
		SecretHashes: []approle.SecretHash{
			{Hash: bcryptHash(t, "expired-secret"), CreatedAt: time.Now(), ExpiresAt: &past},
			{Hash: bcryptHash(t, "current-secret"), CreatedAt: time.Now()},
		},
		Status: approle.StatusActive,
	}
	a := approle.New(&stubStore{role: role})

	if _, err := a.Authenticate(context.Background(), []byte(`{"role_id":"role-uuid","secret_id":"expired-secret"}`)); !errors.Is(err, approle.ErrInvalidSecret) {
		t.Fatalf("expired secret: want ErrInvalidSecret, got %v", err)
	}

	id, err := a.Authenticate(context.Background(), []byte(`{"role_id":"role-uuid","secret_id":"current-secret"}`))
	if err != nil {
		t.Fatalf("current secret: %v", err)
	}
	if id.Subject != "role-uuid" {
		t.Errorf("Subject = %q", id.Subject)
	}
}

func TestAuthenticate_AllSecretHashesExpired_ReturnsErrInvalidSecret(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	role := &approle.Role{
		ID:   "role-uuid",
		Name: "test-role",
		SecretHashes: []approle.SecretHash{
			{Hash: bcryptHash(t, "s1"), ExpiresAt: &past},
			{Hash: bcryptHash(t, "s2"), ExpiresAt: &past},
		},
		Status: approle.StatusActive,
	}
	a := approle.New(&stubStore{role: role})

	if _, err := a.Authenticate(context.Background(), []byte(`{"role_id":"role-uuid","secret_id":"s1"}`)); !errors.Is(err, approle.ErrInvalidSecret) {
		t.Fatalf("want ErrInvalidSecret, got %v", err)
	}
}

func TestAuthenticate_MultipleHashesAnyMatch(t *testing.T) {
	role := &approle.Role{
		ID:    "role-uuid",
		Name:  "test-role",
		Roles: []string{"viewer"},
		SecretHashes: []approle.SecretHash{
			{Hash: bcryptHash(t, "secret-v1")},
			{Hash: bcryptHash(t, "secret-v2")},
			{Hash: bcryptHash(t, "secret-v3")},
		},
		Status: approle.StatusActive,
	}
	a := approle.New(&stubStore{role: role})

	for _, sid := range []string{"secret-v1", "secret-v2", "secret-v3"} {
		if _, err := a.Authenticate(context.Background(), []byte(`{"role_id":"role-uuid","secret_id":"`+sid+`"}`)); err != nil {
			t.Errorf("secret_id %q should authenticate, got: %v", sid, err)
		}
	}
}

func TestAuthenticate_NoCIDRConfig_AllowsAnyIP(t *testing.T) {
	role := validRole(t, "secret-abc")
	role.AllowedCIDRs = nil
	a := approle.New(&stubStore{role: role})

	cred := []byte(`{"role_id":"role-uuid","secret_id":"secret-abc"}`)
	if _, err := a.Authenticate(context.Background(), cred); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
}

func TestAuthenticate_CIDRMatch_Allows(t *testing.T) {
	role := validRole(t, "secret-abc")
	role.AllowedCIDRs = []string{"10.0.0.0/8", "192.168.0.0/16"}
	a := approle.New(&stubStore{role: role})

	ctx := auth.WithSourceIP(context.Background(), "10.5.1.2")
	cred := []byte(`{"role_id":"role-uuid","secret_id":"secret-abc"}`)

	id, err := a.Authenticate(ctx, cred)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Subject != "role-uuid" {
		t.Errorf("Subject = %q", id.Subject)
	}
}

func TestAuthenticate_CIDRNoMatch_ReturnsErrSourceNotAllowed(t *testing.T) {
	role := validRole(t, "secret-abc")
	role.AllowedCIDRs = []string{"10.0.0.0/8"}
	a := approle.New(&stubStore{role: role})

	ctx := auth.WithSourceIP(context.Background(), "192.168.1.1")
	cred := []byte(`{"role_id":"role-uuid","secret_id":"secret-abc"}`)

	if _, err := a.Authenticate(ctx, cred); !errors.Is(err, approle.ErrSourceNotAllowed) {
		t.Fatalf("want ErrSourceNotAllowed, got %v", err)
	}
}

func TestAuthenticate_CIDRSetButNoSourceIPInCtx_Denied(t *testing.T) {
	role := validRole(t, "secret-abc")
	role.AllowedCIDRs = []string{"10.0.0.0/8"}
	a := approle.New(&stubStore{role: role})

	cred := []byte(`{"role_id":"role-uuid","secret_id":"secret-abc"}`)
	if _, err := a.Authenticate(context.Background(), cred); !errors.Is(err, approle.ErrSourceNotAllowed) {
		t.Fatalf("want ErrSourceNotAllowed, got %v", err)
	}
}

func TestAuthenticate_ExpiredHash_BoundaryIsExclusive(t *testing.T) {
	fixed := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return fixed }

	role := &approle.Role{
		ID:    "role-uuid",
		Name:  "test-role",
		Roles: []string{"viewer"},
		SecretHashes: []approle.SecretHash{
			// ExpiresAt EXACTLY at "now" — should be treated as expired.
			{Hash: bcryptHash(t, "secret"), ExpiresAt: &fixed},
		},
		Status: approle.StatusActive,
	}
	a := approle.NewWithClock(&stubStore{role: role}, clock)

	cred := []byte(`{"role_id":"role-uuid","secret_id":"secret"}`)
	if _, err := a.Authenticate(context.Background(), cred); !errors.Is(err, approle.ErrInvalidSecret) {
		t.Fatalf("ExpiresAt == now should be expired; want ErrInvalidSecret, got %v", err)
	}
}

// TestMatchesAnyHash_TimingOracleMitigation verifies the correctness guarantees
// of the constant-iteration matchesAnyHash fix. These cases collectively prove
// that the no-early-return loop produces the same correct outcomes regardless
// of hash position in the slice.
func TestMatchesAnyHash_TimingOracleMitigation(t *testing.T) {
	fixed := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	past := fixed.Add(-1 * time.Hour)
	future := fixed.Add(+1 * time.Hour)

	h := func(plain string) string { return bcryptHash(t, plain) }

	cases := []struct {
		name    string
		hashes  []approle.SecretHash
		secret  string
		wantErr error
	}{
		{
			// (a) Matching hash is NOT the first in the slice — must still succeed.
			name: "match_not_first_position",
			hashes: []approle.SecretHash{
				{Hash: h("other-secret"), CreatedAt: fixed},
				{Hash: h("target-secret"), CreatedAt: fixed},
			},
			secret:  "target-secret",
			wantErr: nil,
		},
		{
			// (b) Multiple valid hashes (rotation window) — any matching secret succeeds.
			name: "multiple_valid_hashes_rotation",
			hashes: []approle.SecretHash{
				{Hash: h("secret-v1"), CreatedAt: fixed},
				{Hash: h("secret-v2"), CreatedAt: fixed},
				{Hash: h("secret-v3"), CreatedAt: fixed},
			},
			secret:  "secret-v2",
			wantErr: nil,
		},
		{
			// (c) No matching hash anywhere → failure.
			name: "no_matching_hash",
			hashes: []approle.SecretHash{
				{Hash: h("correct-one"), CreatedAt: fixed},
				{Hash: h("correct-two"), CreatedAt: fixed},
			},
			secret:  "wrong-secret",
			wantErr: approle.ErrInvalidSecret,
		},
		{
			// (d1) Expired matching hash alone → failure (expired hashes do not count).
			name: "expired_matching_hash_alone",
			hashes: []approle.SecretHash{
				{Hash: h("expired-secret"), CreatedAt: fixed, ExpiresAt: &past},
			},
			secret:  "expired-secret",
			wantErr: approle.ErrInvalidSecret,
		},
		{
			// (d2) Valid hash alongside an expired one → success (valid wins).
			name: "valid_hash_alongside_expired",
			hashes: []approle.SecretHash{
				{Hash: h("expired-secret"), CreatedAt: fixed, ExpiresAt: &past},
				{Hash: h("current-secret"), CreatedAt: fixed, ExpiresAt: &future},
			},
			secret:  "current-secret",
			wantErr: nil,
		},
		{
			// Extra: expired hash is NOT matched even though its plaintext is supplied;
			// the valid hash for a different secret is present but also not matching.
			name: "expired_secret_not_matched_through_expiry_skip",
			hashes: []approle.SecretHash{
				{Hash: h("expired-secret"), CreatedAt: fixed, ExpiresAt: &past},
				{Hash: h("current-secret"), CreatedAt: fixed},
			},
			secret:  "expired-secret",
			wantErr: approle.ErrInvalidSecret,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role := &approle.Role{
				ID:           "role-uuid",
				Name:         "test-role",
				Roles:        []string{"viewer"},
				SecretHashes: tc.hashes,
				Status:       approle.StatusActive,
			}
			a := approle.NewWithClock(&stubStore{role: role}, func() time.Time { return fixed })

			credJSON := `{"role_id":"role-uuid","secret_id":"` + tc.secret + `"}`
			_, err := a.Authenticate(context.Background(), []byte(credJSON))

			if tc.wantErr == nil && err != nil {
				t.Errorf("expected success, got error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestAuthenticate_RoleAttributes_FlowToIdentityMeta(t *testing.T) {
	role := &approle.Role{
		ID:    "role-uuid",
		Name:  "entrypushd-service",
		Roles: []string{"entrypushd"},
		Attributes: map[string]string{
			"allowed_prefixes": "passbox/banking/,passbox/insurance/",
			"env":              "production",
		},
		SecretHashes: []approle.SecretHash{
			{Hash: bcryptHash(t, "s1")},
		},
		Status: approle.StatusActive,
	}
	a := approle.New(&stubStore{role: role})

	id, err := a.Authenticate(context.Background(), []byte(`{"role_id":"role-uuid","secret_id":"s1"}`))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if id.Meta["allowed_prefixes"] != "passbox/banking/,passbox/insurance/" {
		t.Errorf("allowed_prefixes = %q", id.Meta["allowed_prefixes"])
	}
	if id.Meta["env"] != "production" {
		t.Errorf("env = %q", id.Meta["env"])
	}
	if id.Meta["name"] != "entrypushd-service" {
		t.Errorf("name = %q (reserved key should still be populated)", id.Meta["name"])
	}
}
