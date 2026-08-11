package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// M2MTokenTTL is the lifetime of JWTs issued for M2M (machine-to-machine)
// flows. Short relative to Basic Auth (24h) because M2M re-mints on
// demand from the cached Identity in ctx — no client refresh round-trip.
const M2MTokenTTL = 15 * time.Minute

// ErrNilIdentity is returned by MintM2MJWT when given a nil Identity.
var ErrNilIdentity = errors.New("auth: nil identity")

// M2MAudiences returns the JWT `aud` claim used for all M2M tokens in
// Fase 2.1.a. Same token valid against both nbox HTTP API and entrypushd
// gRPC. A fresh slice is returned each call so callers cannot mutate
// the canonical set. aud enforcement (rejecting tokens that don't
// include the receiver's name) arrives in Fase 2.1.c.
func M2MAudiences() jwt.ClaimStrings {
	return jwt.ClaimStrings{"nbox", "entrypushd"}
}

// MintM2MJWT issues a JWT for an authenticated M2M identity. Audience
// is always M2MAudiences() in this phase.
//
// Reserved Meta keys consumed here (NOT flowed verbatim into the
// `attributes` claim):
//   - "name" → Claims.Name (falls back to Identity.Subject when absent)
//
// All other Meta entries populate Claims.Attributes (ABAC seed).
func MintM2MJWT(identity *Identity, hmacKey []byte) (string, error) {
	if identity == nil {
		return "", ErrNilIdentity
	}
	now := time.Now()

	name := identity.Meta["name"]
	if name == "" {
		name = identity.Subject
	}

	var attrs map[string]string
	for k, v := range identity.Meta {
		if k == "name" {
			continue
		}
		if attrs == nil {
			attrs = make(map[string]string)
		}
		attrs[k] = v
	}

	claims := &Claims{
		Username:   identity.Subject,
		Name:       name,
		Roles:      identity.Roles,
		Attributes: attrs,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(M2MTokenTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Audience:  M2MAudiences(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(hmacKey)
}
