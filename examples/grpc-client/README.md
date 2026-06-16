# gRPC Watch — client guide

`entrypushd` exposes the `stream.v1.KVStream` service with a
server-streaming `Watch` method. This doc is the shared wire contract;
runnable examples live per language:

| Folder | What |
|---|---|
| [`go/`](go/README.md) | Go client (auth + HPKE decrypt) |
| [`python/`](python/README.md) | `watch.py` — runnable with `uv run` |
| [`shell/`](shell/watch.sh) | `watch.sh` — grpcurl smoke test |

**In short:**

- **Auth required** on every RPC (reflection included), via `authorization`
  metadata. No TLS yet.
- **Snapshot + deltas**: connecting with a `prefixes` filter delivers the
  current state first, then live changes, with no gap.
- **Vault encrypted**: `passbox/*` values arrive sealed to your public key
  if you present one — see [Vault](#vault-encrypted-delivery).

## Authentication

The agent presents credentials in the `authorization` metadata:

```
authorization: AppRole <base64(JSON{role_id, secret_id})>
```

- `role_id` / `secret_id` are UUIDs issued by the admin.
- The scheme tag (`AppRole`) selects the authenticator. `AWS-STS` exists
  with the same syntax (see below).

### Get credentials (admin)

```bash
nbox-cli approle generate watcher-agent --opa-role entrypushd
```

Outputs `role_id`, `secret_id` (distribute over a secure channel), and a
JSON entry for `NBOX_APPROLE_ROLES`. Rotate with `nbox-cli approle
rotate-secret` (append the new `SecretHash`, distribute the new
`secret_id`, then drop the old hash after a grace window).

### Auth error codes

| Status | Cause |
|---|---|
| `Unauthenticated` "missing authorization metadata" | no `authorization` metadata |
| `Unauthenticated` "malformed authorization metadata" | missing space between scheme and body, or empty body |
| `Unauthenticated` "credential is not valid base64" | body after the scheme isn't valid base64 |
| `Unauthenticated` "unsupported auth scheme: …" | scheme tag not in the registry |
| `Unauthenticated` "invalid credentials" | unknown/disabled role, wrong secret, or IP outside `allowed_cidrs` |
| `InvalidArgument` "invalid credential format" | decoded JSON lacks `role_id`/`secret_id` |
| `Unavailable` "approle auth is disabled" | kill switch (`NBOX_APPROLE_DISABLED=true`) |

All credential failures collapse to `Unauthenticated "invalid
credentials"` — which check failed is never leaked.

### AWS-STS scheme

For agents inside AWS, AWS-STS auth avoids distributing `role_id`/`secret_id`:
the agent sigv4-signs a `GetCallerIdentity` request; entrypushd forwards it
to STS and matches the returned ARN against `NBOX_AWS_ARN_MAP`.

```
authorization: AWS-STS <base64(JSON{iam_http_request_method, iam_request_url, iam_request_body, iam_request_headers})>
```

The credential is built with `internal/auth/awssts.BuildCredential` (Go) or
a boto3 `SigV4Auth` over `Action=GetCallerIdentity` (Python). End-to-end
setup, testing, and troubleshooting: [AWSSTS-TESTING.md](AWSSTS-TESTING.md).

## Contract

Proto: [`proto/kvstream.proto`](../../proto/kvstream.proto)

```protobuf
service KVStream {
  rpc Watch(WatchRequest) returns (stream Event);
}

message WatchRequest {
  repeated string prefixes = 1;  // filter by subject prefix (HasPrefix, OR)
  repeated string types    = 2;  // filter by type (exact or wildcard "*", OR)
}

message Event {
  string id = 1;
  string type = 2;                       // "nbox.entry.upserted", etc.
  string source = 3;
  string subject = 4;                    // entry key
  int64 time_unix_ms = 5;
  bytes data = 6;                        // JSON payload, or HPKE ciphertext for vault
  map<string, string> extensions = 7;
}
```

Filter semantics: both empty → all events; both set → AND (match a prefix
AND a type); within each list → OR.

## Vault encrypted delivery

`passbox/*` values never travel in clear. Present your public key in the
`Watch` metadata and entrypushd seals each value to it (HPKE) — only you
can open it. Without a key, the value arrives masked (`*****`).

The agent: (1) generates an X25519 keypair at boot (private never leaves
the process, reused across reconnects); (2) sends it in the metadata
alongside `authorization`:

```
x-vault-pubkey:         base64(32-byte X25519 public key)
x-vault-instance-nonce: a boot nonce (e.g. a UUID)
```

(3) decrypts events whose `extensions["encrypted"] == "hpke"`; the rest
arrive in clear.

**To decrypt:**

| | Value |
|---|---|
| Suite | `DHKEM(X25519)` + `HKDF-SHA256` + `AES-256-GCM` |
| `info` | `"nbox/vault/v1\|" + event.subject` |
| `data` | `enc ‖ ciphertext` — for X25519, `enc` = first **32 bytes** |

Code: [`go/`](go/README.md) (`crypto/hpke`), [`python/`](python/README.md)
(`pyhpke`). `grpcurl` can't decrypt HPKE — with it, vault values stay masked.

## Errors and reconnection

- **Auth failures (`Unauthenticated`)** are terminal: don't retry with the
  same credentials. If `secret_id` rotated, refresh and reconnect.
- **Graceful shutdown**: the server closes the stream; reconnect. Auth is
  re-validated on each reconnect.
- **Subscriber buffer**: 20 events. A slow consumer drops overflow
  silently — no client ack, best-effort delivery.
- **Snapshot-on-connect**: with a `prefixes` filter you get the current
  state first, then deltas. On reconnect the snapshot re-delivers, so
  "replay" comes from the snapshot. Apply last-write-wins by `subject`.
  (Needs `ENTRYPUSHD_NBOX_URL` set; empty → deltas only.)
- **Vault key across reconnects**: keep the same X25519 keypair per process
  and re-send the same `x-vault-pubkey`. Any replica learns it from the
  handshake and re-seals the snapshot to it — no shared state to migrate.
  Rotate the key by reconnecting with a new keypair.

## Operational security

- **Never log `secret_id`** (the server doesn't either).
- Distribute credentials over a secure channel (k8s secret, vault, CI
  env) — no commits.
- `allowed_cidrs`: connecting from a non-allowed IP fails with
  `Unauthenticated` even with valid credentials.
