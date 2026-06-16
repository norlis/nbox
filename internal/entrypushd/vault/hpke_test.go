package vault_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"nbox/internal/entrypushd/vault"
)

func newKeypair(t *testing.T) (*ecdh.PrivateKey, *ecdh.PublicKey) {
	t.Helper()
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv, priv.PublicKey()
}

func TestSealOpen_RoundTrip(t *testing.T) {
	priv, pub := newKeypair(t)
	plaintext := []byte("super-secret-value")

	sealed, err := vault.Seal(pub, "passbox/banking/api", plaintext)
	require.NoError(t, err)
	require.NotEqual(t, plaintext, sealed, "ciphertext must differ from plaintext")

	got, err := vault.Open(priv, "passbox/banking/api", sealed)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}

func TestOpen_WrongKeyFails(t *testing.T) {
	_, pub := newKeypair(t)
	otherPriv, _ := newKeypair(t)

	sealed, err := vault.Seal(pub, "passbox/k", []byte("v"))
	require.NoError(t, err)

	_, err = vault.Open(otherPriv, "passbox/k", sealed)
	require.Error(t, err, "decrypting with a different private key must fail")
}

func TestOpen_WrongInfoFails(t *testing.T) {
	priv, pub := newKeypair(t)
	sealed, err := vault.Seal(pub, "passbox/k1", []byte("v"))
	require.NoError(t, err)

	_, err = vault.Open(priv, "passbox/k2", sealed)
	require.Error(t, err, "info binding: opening under a different key must fail")
}

func TestOpen_TamperFails(t *testing.T) {
	priv, pub := newKeypair(t)
	sealed, err := vault.Seal(pub, "passbox/k", []byte("value"))
	require.NoError(t, err)

	sealed[len(sealed)-1] ^= 0xFF // flip last ciphertext byte
	_, err = vault.Open(priv, "passbox/k", sealed)
	require.Error(t, err, "AEAD must reject tampered ciphertext")
}
