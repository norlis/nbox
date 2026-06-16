package vault

import (
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// realm namespaces the client_id hash to this feature/version.
const realm = "nbox/vault/v1"

// Fingerprint is a stable, hex-free short id of a public key
// (base32 of SHA-256 over the raw key bytes, no padding).
func Fingerprint(pub *ecdh.PublicKey) string {
	sum := sha256.Sum256(pub.Bytes())
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:])
}

// DeriveClientID returns a stable, non-forgeable id for an agent session.
// It binds the authenticated subject, the agent-supplied instance nonce,
// and the public key fingerprint. entrypushd recomputes it from
// authenticated material — the client never chooses it.
func DeriveClientID(subject, nonce string, pub *ecdh.PublicKey) string {
	h := sha256.New()
	// length-free domain separation via "|" joins of fixed-order fields.
	_, _ = h.Write([]byte(strings.Join([]string{realm, subject, nonce, Fingerprint(pub)}, "|")))
	sum := h.Sum(nil)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum)
}
