package store_test

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"nbox/internal/auth"
	"nbox/internal/auth/store"
)

// bcryptHash is a helper to generate a bcrypt hash for a known password.
func bcryptHash(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return string(h)
}

// credentialsJSON builds a minimal JSON credential blob understood by NewInMemory.
func credentialsJSON(t *testing.T, username, password string, roles []string) []byte {
	t.Helper()
	hash := bcryptHash(t, password)
	rolesJSON := `["` + roles[0] + `"]`
	for _, r := range roles[1:] {
		rolesJSON = rolesJSON[:len(rolesJSON)-1] + `,"` + r + `"]`
	}
	return []byte(`{"` + username + `":{"password":"` + hash + `","roles":` + rolesJSON + `,"status":"active"}}`)
}

// ---- ValidatePassword: happy path -----------------------------------------------

func TestValidatePassword_CorrectCredentials_ReturnsUser(t *testing.T) {
	creds := credentialsJSON(t, "alice", "s3cret", []string{"admin"})
	s, err := store.NewInMemory(creds)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	user, err := s.ValidatePassword(context.Background(), "alice", "s3cret")
	if err != nil {
		t.Fatalf("ValidatePassword: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("Username = %q, want %q", user.Username, "alice")
	}
}

// ---- ValidatePassword: wrong password -----------------------------------------------

func TestValidatePassword_WrongPassword_ReturnsErrInvalidPassword(t *testing.T) {
	creds := credentialsJSON(t, "alice", "s3cret", []string{"admin"})
	s, err := store.NewInMemory(creds)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	_, err = s.ValidatePassword(context.Background(), "alice", "wrong")
	if !errors.Is(err, auth.ErrInvalidPassword) {
		t.Errorf("err = %v, want ErrInvalidPassword", err)
	}
}

// ---- ValidatePassword: unknown username -----------------------------------------

func TestValidatePassword_UnknownUsername_ReturnsErrUserNotFound(t *testing.T) {
	creds := credentialsJSON(t, "alice", "s3cret", []string{"admin"})
	s, err := store.NewInMemory(creds)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	_, err = s.ValidatePassword(context.Background(), "nobody", "anything")
	if !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

// TestValidatePassword_UnknownUsername_DoesNotPanic confirms the dummy-hash path
// runs bcrypt without panicking (the timing-equalisation path).
func TestValidatePassword_UnknownUsername_DoesNotPanic(t *testing.T) {
	creds := credentialsJSON(t, "alice", "s3cret", []string{"admin"})
	s, err := store.NewInMemory(creds)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	// Should not panic; the dummy bcrypt path must be exercised.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ValidatePassword panicked: %v", r)
		}
	}()

	_, _ = s.ValidatePassword(context.Background(), "nonexistent", "testpassword")
}

// ---- FindByUsername ---------------------------------------------------------------

func TestFindByUsername_KnownUser_ReturnsUser(t *testing.T) {
	creds := credentialsJSON(t, "bob", "pw", []string{"viewer"})
	s, err := store.NewInMemory(creds)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	user, err := s.FindByUsername(context.Background(), "bob")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if user.Username != "bob" {
		t.Errorf("Username = %q, want %q", user.Username, "bob")
	}
}

func TestFindByUsername_UnknownUser_ReturnsErrUserNotFound(t *testing.T) {
	creds := credentialsJSON(t, "bob", "pw", []string{"viewer"})
	s, err := store.NewInMemory(creds)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	_, err = s.FindByUsername(context.Background(), "unknown")
	if !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

// ---- NewInMemory: JSON parsing ---------------------------------------------------

func TestNewInMemory_InvalidJSON_ReturnsError(t *testing.T) {
	_, err := store.NewInMemory([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
