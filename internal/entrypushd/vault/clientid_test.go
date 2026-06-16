package vault_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"nbox/internal/entrypushd/vault"
)

func TestDeriveClientID_Deterministic(t *testing.T) {
	_, pub := newKeypair(t)
	a := vault.DeriveClientID("role-123", "nonce-abc", pub)
	b := vault.DeriveClientID("role-123", "nonce-abc", pub)
	require.Equal(t, a, b, "same inputs => same id")
	require.NotEmpty(t, a)
}

func TestDeriveClientID_SensitiveToEachComponent(t *testing.T) {
	_, pub1 := newKeypair(t)
	_, pub2 := newKeypair(t)
	base := vault.DeriveClientID("role-1", "nonce-1", pub1)

	require.NotEqual(t, base, vault.DeriveClientID("role-2", "nonce-1", pub1), "subject changes id")
	require.NotEqual(t, base, vault.DeriveClientID("role-1", "nonce-2", pub1), "nonce changes id")
	require.NotEqual(t, base, vault.DeriveClientID("role-1", "nonce-1", pub2), "pubkey changes id")
}

func TestFingerprint_StableAndKeyed(t *testing.T) {
	_, pub1 := newKeypair(t)
	_, pub2 := newKeypair(t)
	require.Equal(t, vault.Fingerprint(pub1), vault.Fingerprint(pub1))
	require.NotEqual(t, vault.Fingerprint(pub1), vault.Fingerprint(pub2))
}
