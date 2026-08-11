# Go client

Watch client with AppRole auth and HPKE decryption of vault (`passbox/*`)
values. See [../README.md](../README.md) for the wire contract.

## Setup

```bash
go get google.golang.org/grpc google.golang.org/protobuf
# generate stubs from the proto into your module:
protoc \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/kvstream.proto
```

## Example

HPKE decryption uses `crypto/hpke` (Go 1.26+). For older Go, swap in
`github.com/cloudflare/circl/hpke` with the same suite.

```go
package main

import (
	"context"
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	streamv1 "yourmodule/gen/stream/v1"
)

func authHeader(roleID, secretID string) string {
	body, _ := json.Marshal(map[string]string{"role_id": roleID, "secret_id": secretID})
	return "AppRole " + base64.StdEncoding.EncodeToString(body)
}

// decrypt returns the value: HPKE-open for vault keys, plain otherwise.
func decrypt(priv *ecdh.PrivateKey, evt *streamv1.Event) ([]byte, error) {
	if evt.Extensions["encrypted"] != "hpke" {
		return evt.Data, nil
	}
	sk, err := hpke.NewDHKEMPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	info := []byte("nbox/vault/v1|" + evt.Subject)
	return hpke.Open(sk, hpke.HKDFSHA256(), hpke.AES256GCM(), info, evt.Data)
}

func main() {
	// ephemeral X25519 keypair; the private key never leaves the process.
	priv, _ := ecdh.X25519().GenerateKey(rand.Reader)
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())

	conn, err := grpc.NewClient("localhost:9337",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"authorization", authHeader(os.Getenv("NBOX_ROLE_ID"), os.Getenv("NBOX_SECRET_ID")),
		"x-vault-pubkey", pubB64,
		"x-vault-instance-nonce", "watch-go",
	)

	stream, err := streamv1.NewKVStreamClient(conn).Watch(ctx,
		&streamv1.WatchRequest{Prefixes: []string{"passbox/"}})
	if err != nil {
		log.Fatal(err)
	}
	for {
		evt, err := stream.Recv()
		if err != nil {
			log.Fatal(err)
		}
		val, err := decrypt(priv, evt)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("[%s] %s = %q\n", evt.Type, evt.Subject, val)
	}
}
```
