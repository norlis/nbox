package grpc

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestClientKeyFromContext_OK(t *testing.T) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())

	md := metadata.New(map[string]string{
		"x-vault-pubkey":         pubB64,
		"x-vault-instance-nonce": "nonce-xyz",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	pub, nonce, ok := clientKeyFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, "nonce-xyz", nonce)
	require.Equal(t, priv.PublicKey().Bytes(), pub.Bytes())
}

func TestClientKeyFromContext_MissingPubkey(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{"x-vault-instance-nonce": "n"}))
	_, _, ok := clientKeyFromContext(ctx)
	require.False(t, ok)
}

func TestClientKeyFromContext_BadBase64(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{"x-vault-pubkey": "!!notb64!!"}))
	_, _, ok := clientKeyFromContext(ctx)
	require.False(t, ok)
}

func TestClientKeyFromContext_WrongKeySize(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.New(map[string]string{"x-vault-pubkey": base64.StdEncoding.EncodeToString([]byte("short"))}))
	_, _, ok := clientKeyFromContext(ctx)
	require.False(t, ok)
}
