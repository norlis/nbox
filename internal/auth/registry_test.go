package auth_test

import (
	"context"
	"testing"

	"nbox/internal/auth"
)

// stubAuthenticator is a minimal implementation for registry testing.
type stubAuthenticator struct {
	method string
}

func (s *stubAuthenticator) Method() string { return s.method }
func (s *stubAuthenticator) Authenticate(_ context.Context, _ []byte) (*auth.Identity, error) {
	return &auth.Identity{Method: s.method, Subject: "stub"}, nil
}

func TestRegistry_GetReturnsRegisteredAuth(t *testing.T) {
	a := &stubAuthenticator{method: "approle"}
	reg := auth.NewRegistry(a)

	got, ok := reg.Get("approle")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != auth.Authenticator(a) {
		t.Errorf("got %p, want %p", got, a)
	}
}

func TestRegistry_GetReturnsFalseForUnknown(t *testing.T) {
	reg := auth.NewRegistry(&stubAuthenticator{method: "approle"})

	if _, ok := reg.Get("aws-sts"); ok {
		t.Error("expected ok=false for unknown method")
	}
}

func TestRegistry_DuplicateMethod_LastWins(t *testing.T) {
	first := &stubAuthenticator{method: "approle"}
	second := &stubAuthenticator{method: "approle"}
	reg := auth.NewRegistry(first, second)

	got, ok := reg.Get("approle")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got == auth.Authenticator(first) {
		t.Error("first should have been overwritten by second")
	}
	if got != auth.Authenticator(second) {
		t.Errorf("got %p, want %p (second)", got, second)
	}
}
