package auth_test

import (
	"context"
	"testing"

	"nbox/internal/auth"
)

func TestWithIdentity_RoundTrip(t *testing.T) {
	id := &auth.Identity{Method: "approle", Subject: "role-uuid"}
	ctx := auth.WithIdentity(context.Background(), id)

	got, ok := auth.IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != id {
		t.Errorf("got %p, want %p", got, id)
	}
}

func TestIdentityFromContext_MissingReturnsFalse(t *testing.T) {
	if _, ok := auth.IdentityFromContext(context.Background()); ok {
		t.Error("expected ok=false on empty ctx")
	}
}

func TestWithM2MToken_RoundTrip(t *testing.T) {
	ctx := auth.WithM2MToken(context.Background(), "eyJ.signed.token")

	got, ok := auth.M2MTokenFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != "eyJ.signed.token" {
		t.Errorf("got %q", got)
	}
}

func TestM2MTokenFromContext_MissingReturnsFalse(t *testing.T) {
	if _, ok := auth.M2MTokenFromContext(context.Background()); ok {
		t.Error("expected ok=false on empty ctx")
	}
}
