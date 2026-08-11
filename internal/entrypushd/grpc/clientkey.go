package grpc

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"

	"google.golang.org/grpc/metadata"
)

const (
	mdPubkey = "x-vault-pubkey"         // base64(std) of the 32-byte X25519 public key
	mdNonce  = "x-vault-instance-nonce" // per-instance boot nonce
)

// clientKeyFromContext extracts the agent's X25519 public key and instance
// nonce from the incoming gRPC metadata. Returns ok=false if absent or
// malformed (the caller then treats the stream as plaintext-only / non-vault).
func clientKeyFromContext(ctx context.Context) (pub *ecdh.PublicKey, nonce string, ok bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, "", false
	}
	pkVals := md.Get(mdPubkey)
	if len(pkVals) == 0 {
		return nil, "", false
	}
	raw, err := base64.StdEncoding.DecodeString(pkVals[0])
	if err != nil {
		return nil, "", false
	}
	pub, err = ecdh.X25519().NewPublicKey(raw) // rejects wrong-size keys
	if err != nil {
		return nil, "", false
	}
	if nv := md.Get(mdNonce); len(nv) > 0 {
		nonce = nv[0]
	}
	return pub, nonce, true
}
