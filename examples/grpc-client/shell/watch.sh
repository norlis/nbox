#!/usr/bin/env bash
#
# Smoke-test the entrypushd Watch stream with grpcurl.
#
# Every RPC needs auth metadata (reflection included). Vault values
# (passbox/*) arrive MASKED (*****) here — grpcurl can't decrypt HPKE;
# use ../python/watch.py to read them.
#
# Requires: grpcurl. Set credentials, then run:
#   export NBOX_ROLE_ID=40a87867-d7f5-4c8a-80b6-914c9a7667d2  NBOX_SECRET_ID=b981b147-e7d7-426d-a19a-5173636c3b18
#   ./watch.sh                 # subscribe to everything
#   ./watch.sh passbox/        # filter by prefix
#
# Env: NBOX_GRPC (default localhost:9337)
#
set -euo pipefail

GRPC="${NBOX_GRPC:-localhost:9337}"
: "${NBOX_ROLE_ID:?set NBOX_ROLE_ID}" "${NBOX_SECRET_ID:?set NBOX_SECRET_ID}"

CRED_B64=$(printf '{"role_id":"%s","secret_id":"%s"}' "$NBOX_ROLE_ID" "$NBOX_SECRET_ID" | base64 | tr -d '\n')
AUTH="authorization: AppRole $CRED_B64"

if [[ -n "${1:-}" ]]; then
  DATA=$(printf '{"prefixes":["%s"]}' "$1")
else
  DATA='{}'
fi

# Reflection helpers (optional):
#   grpcurl -plaintext -H "$AUTH" "$GRPC" list
#   grpcurl -plaintext -H "$AUTH" "$GRPC" describe stream.v1.KVStream

grpcurl -plaintext -H "$AUTH" -d "$DATA" "$GRPC" stream.v1.KVStream/Watch
