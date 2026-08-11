// Package vault implements HPKE (RFC 9180) sealing of secret values to a
// per-agent X25519 public key. Suite v1: DHKEM(X25519, HKDF-SHA256) +
// HKDF-SHA256 + AES-256-GCM. It is a leaf package: it imports nothing
// from internal/entrypushd, so internal/entrypushd/grpc can import it
// without an import cycle.
package vault

import (
	"crypto/ecdh"
	"crypto/hpke"
	"fmt"
)

// infoPrefix binds a sealed value to its key, so a ciphertext for one key
// cannot be opened as another. Must match on Seal and Open.
const infoPrefix = "nbox/vault/v1|"

func info(key string) []byte { return []byte(infoPrefix + key) }

// suite v1 components (RFC 9180 interop-standard).
func kdf() hpke.KDF   { return hpke.HKDFSHA256() }
func aead() hpke.AEAD { return hpke.AES256GCM() }

// Seal encrypts plaintext to the agent's X25519 public key. Returns the
// HPKE single-shot output (encapsulated key ‖ ciphertext).
func Seal(pub *ecdh.PublicKey, key string, plaintext []byte) ([]byte, error) {
	pk, err := hpke.NewDHKEMPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("vault seal: public key: %w", err)
	}
	sealed, err := hpke.Seal(pk, kdf(), aead(), info(key), plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault seal: %w", err)
	}
	return sealed, nil
}

// Open decrypts a value sealed by Seal. Used by tests and the agent SDK.
func Open(priv *ecdh.PrivateKey, key string, sealed []byte) ([]byte, error) {
	sk, err := hpke.NewDHKEMPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("vault open: private key: %w", err)
	}
	plaintext, err := hpke.Open(sk, kdf(), aead(), info(key), sealed)
	if err != nil {
		return nil, fmt.Errorf("vault open: %w", err)
	}
	return plaintext, nil
}
