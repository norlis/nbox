package approle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"nbox/internal/auth/approle"
)

func TestInMemory_ParsesValidJSON(t *testing.T) {
	raw := []byte(`[
		{
			"id": "ce3f9e0a-1234-5678-9abc-def012345678",
			"name": "entrypushd-service",
			"roles": ["entrypushd"],
			"secret_hashes": [
				{"hash": "$2a$10$abc", "created_at": "2026-05-19T00:00:00Z"}
			],
			"status": "active"
		}
	]`)

	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	role, err := s.GetRole(context.Background(), "ce3f9e0a-1234-5678-9abc-def012345678")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Name != "entrypushd-service" {
		t.Errorf("Name = %q, want %q", role.Name, "entrypushd-service")
	}
	if !slices.Equal(role.Roles, []string{"entrypushd"}) {
		t.Errorf("Roles = %v, want [entrypushd]", role.Roles)
	}
	if len(role.SecretHashes) != 1 {
		t.Errorf("len(SecretHashes) = %d, want 1", len(role.SecretHashes))
	}
	if role.Status != approle.StatusActive {
		t.Errorf("Status = %q, want %q", role.Status, approle.StatusActive)
	}
}

func TestInMemory_EmptyInput_NoRoles(t *testing.T) {
	s, err := approle.NewInMemory(nil)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	if _, err := s.GetRole(context.Background(), "anything"); !errors.Is(err, approle.ErrRoleNotFound) {
		t.Fatalf("want ErrRoleNotFound, got %v", err)
	}
}

func TestInMemory_InvalidJSON_ReturnsError(t *testing.T) {
	if _, err := approle.NewInMemory([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestInMemory_RoleWithEmptyID_ReturnsError(t *testing.T) {
	raw := []byte(`[{"id": "", "name": "x", "roles": [], "secret_hashes": [], "status": "active"}]`)
	_, err := approle.NewInMemory(raw)
	if err == nil {
		t.Fatal("expected error for empty id")
	}
	if !strings.Contains(err.Error(), "empty id") {
		t.Errorf("error %q should mention 'empty id'", err)
	}
}

func TestInMemory_DefaultStatusIsActive(t *testing.T) {
	raw := []byte(`[{"id": "abc", "name": "x", "roles": [], "secret_hashes": []}]`)
	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	role, err := s.GetRole(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Status != approle.StatusActive {
		t.Errorf("Status = %q, want %q (default)", role.Status, approle.StatusActive)
	}
}

func TestInMemory_GetRole_UnknownRoleID(t *testing.T) {
	raw := []byte(`[{"id": "known", "name": "x", "roles": [], "secret_hashes": [], "status": "active"}]`)
	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	if _, err := s.GetRole(context.Background(), "unknown"); !errors.Is(err, approle.ErrRoleNotFound) {
		t.Fatalf("want ErrRoleNotFound, got %v", err)
	}
}

func TestInMemory_ParsesAttributes(t *testing.T) {
	raw := []byte(`[
		{
			"id": "abc",
			"name": "entrypushd",
			"roles": ["entrypushd"],
			"attributes": {
				"allowed_prefixes": "passbox/banking/",
				"env": "production"
			},
			"secret_hashes": [],
			"status": "active"
		}
	]`)
	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	role, err := s.GetRole(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Attributes["allowed_prefixes"] != "passbox/banking/" {
		t.Errorf("allowed_prefixes = %q", role.Attributes["allowed_prefixes"])
	}
	if role.Attributes["env"] != "production" {
		t.Errorf("env = %q", role.Attributes["env"])
	}
}

func TestInMemory_AttributesOptional(t *testing.T) {
	raw := []byte(`[{"id": "abc", "name": "x", "roles": [], "secret_hashes": [], "status": "active"}]`)
	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	role, err := s.GetRole(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Attributes != nil {
		t.Errorf("Attributes = %v, want nil", role.Attributes)
	}
}

func TestInMemory_ParsesAllowedCIDRs(t *testing.T) {
	raw := []byte(`[
		{
			"id": "abc",
			"name": "entrypushd",
			"roles": ["entrypushd"],
			"allowed_cidrs": ["10.0.0.0/8", "172.16.0.0/12"],
			"secret_hashes": [],
			"status": "active"
		}
	]`)
	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	role, err := s.GetRole(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	want := []string{"10.0.0.0/8", "172.16.0.0/12"}
	if !slices.Equal(role.AllowedCIDRs, want) {
		t.Errorf("AllowedCIDRs = %v, want %v", role.AllowedCIDRs, want)
	}
}

func TestInMemory_DuplicateRoleID_LastWins(t *testing.T) {
	raw := []byte(`[
		{"id": "dup", "name": "first", "roles": [], "secret_hashes": [], "status": "active"},
		{"id": "dup", "name": "second", "roles": [], "secret_hashes": [], "status": "active"}
	]`)
	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	role, err := s.GetRole(context.Background(), "dup")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Name != "second" {
		t.Errorf("Name = %q, want %q (last occurrence)", role.Name, "second")
	}
}

func TestInMemory_ParsesSecretHashTimeFields(t *testing.T) {
	raw := []byte(`[
		{
			"id": "abc",
			"name": "x",
			"roles": [],
			"secret_hashes": [
				{
					"hash": "$2a$10$x",
					"created_at": "2026-05-19T12:34:56Z",
					"expires_at": "2026-06-19T12:34:56Z"
				},
				{
					"hash": "$2a$10$y",
					"created_at": "2026-05-20T00:00:00Z"
				}
			],
			"status": "active"
		}
	]`)
	s, err := approle.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	role, err := s.GetRole(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if len(role.SecretHashes) != 2 {
		t.Fatalf("len(SecretHashes) = %d, want 2", len(role.SecretHashes))
	}

	first := role.SecretHashes[0]
	if first.CreatedAt.IsZero() {
		t.Error("first.CreatedAt should not be zero")
	}
	if first.ExpiresAt == nil {
		t.Fatal("first.ExpiresAt should not be nil")
	}
	if !first.ExpiresAt.After(first.CreatedAt) {
		t.Errorf("ExpiresAt %v should be after CreatedAt %v", first.ExpiresAt, first.CreatedAt)
	}

	second := role.SecretHashes[1]
	if second.ExpiresAt != nil {
		t.Errorf("second.ExpiresAt = %v, want nil (omitempty)", *second.ExpiresAt)
	}
}
