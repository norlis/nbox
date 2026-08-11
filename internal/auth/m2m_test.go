package auth_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"nbox/internal/auth"
)

const testHmacKey = "test-secret-key-32-bytes-long!!!"

func parseToken(t *testing.T, token string) *auth.Claims {
	t.Helper()
	claims := &auth.Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
		return []byte(testHmacKey), nil
	})
	if err != nil {
		t.Fatalf("parse JWT: %v", err)
	}
	return claims
}

func TestMintM2MJWT_SubjectAndRoles(t *testing.T) {
	id := &auth.Identity{
		Method:  "approle",
		Subject: "role-uuid",
		Roles:   []string{"entrypushd", "viewer"},
	}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	if claims.Username != "role-uuid" {
		t.Errorf("Username = %q", claims.Username)
	}
	if !slices.Equal(claims.Roles, []string{"entrypushd", "viewer"}) {
		t.Errorf("Roles = %v", claims.Roles)
	}
}

func TestMintM2MJWT_NameDefaultsToSubject(t *testing.T) {
	id := &auth.Identity{Method: "approle", Subject: "role-uuid"}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	if claims.Name != "role-uuid" {
		t.Errorf("Name = %q, want subject as default", claims.Name)
	}
}

func TestMintM2MJWT_NameFromMeta(t *testing.T) {
	id := &auth.Identity{
		Method:  "approle",
		Subject: "role-uuid",
		Meta:    map[string]string{"name": "watcher-agent"},
	}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	if claims.Name != "watcher-agent" {
		t.Errorf("Name = %q", claims.Name)
	}
}

func TestMintM2MJWT_AttributesFromMetaExcludesName(t *testing.T) {
	id := &auth.Identity{
		Method:  "approle",
		Subject: "role-uuid",
		Meta: map[string]string{
			"name":             "watcher-agent",
			"allowed_prefixes": "passbox/banking/",
			"env":              "production",
		},
	}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	if claims.Name != "watcher-agent" {
		t.Errorf("Name = %q", claims.Name)
	}
	if _, ok := claims.Attributes["name"]; ok {
		t.Error("Attributes should NOT contain 'name' (reserved)")
	}
	if claims.Attributes["allowed_prefixes"] != "passbox/banking/" {
		t.Errorf("allowed_prefixes = %q", claims.Attributes["allowed_prefixes"])
	}
	if claims.Attributes["env"] != "production" {
		t.Errorf("env = %q", claims.Attributes["env"])
	}
}

func TestMintM2MJWT_NoAttributesWhenMetaIsEmpty(t *testing.T) {
	id := &auth.Identity{Method: "approle", Subject: "role-uuid"}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	if len(claims.Attributes) != 0 {
		t.Errorf("Attributes = %v, want empty/nil", claims.Attributes)
	}
}

func TestMintM2MJWT_AudienceIsNboxAndEntrypushd(t *testing.T) {
	id := &auth.Identity{Method: "approle", Subject: "role-uuid"}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	want := []string{"nbox", "entrypushd"}
	if !slices.Equal([]string(claims.Audience), want) {
		t.Errorf("Audience = %v, want %v", claims.Audience, want)
	}
}

func TestMintM2MJWT_TTLIs15Minutes(t *testing.T) {
	id := &auth.Identity{Method: "approle", Subject: "role-uuid"}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl >= 16*time.Minute || ttl <= 14*time.Minute {
		t.Errorf("TTL = %v, want ~15min", ttl)
	}
}

func TestMintM2MJWT_SignedWithProvidedKey(t *testing.T) {
	id := &auth.Identity{Method: "approle", Subject: "role-uuid"}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	// Parse with the WRONG key — should fail.
	_, err = jwt.ParseWithClaims(token, &auth.Claims{}, func(_ *jwt.Token) (any, error) {
		return []byte("wrong-key"), nil
	})
	if err == nil {
		t.Error("expected verification failure with wrong key")
	}
}

func TestMintM2MJWT_EmptyRoles(t *testing.T) {
	id := &auth.Identity{
		Method:  "approle",
		Subject: "role-uuid",
		Roles:   nil, // explicitly empty
	}
	token, err := auth.MintM2MJWT(id, []byte(testHmacKey))
	if err != nil {
		t.Fatalf("MintM2MJWT: %v", err)
	}
	claims := parseToken(t, token)
	if len(claims.Roles) != 0 {
		t.Errorf("Roles = %v, want empty", claims.Roles)
	}
}

func TestMintM2MJWT_NilIdentity_ReturnsError(t *testing.T) {
	_, err := auth.MintM2MJWT(nil, []byte(testHmacKey))
	if !errors.Is(err, auth.ErrNilIdentity) {
		t.Errorf("want ErrNilIdentity, got %v", err)
	}
}
