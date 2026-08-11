# /// script
# requires-python = ">=3.14"
# dependencies = [
#     "grpcio",
#     "grpcio-tools",
#     "cryptography",
#     "pyhpke",
#     "boto3",
# ]
# ///
"""nbox/entrypushd Watch client.

Subscribes to a prefix, HPKE-decrypts vault (passbox/*) values, and prints
each event. Auto-reconnects on stream loss (same keypair across retries, so
entrypushd re-seals the snapshot to it).

    # AppRole (default)
    export NBOX_ROLE_ID=...  NBOX_SECRET_ID=...
    uv run watch.py                    # default prefix: passbox/
    uv run watch.py development/ qa/   # custom prefixes

    # AWS-STS (agent inside AWS, uses ambient IAM credentials)
    export NBOX_AUTH=aws-sts
    uv run watch.py

Env: NBOX_GRPC (default localhost:9337), NBOX_AUTH (approle|aws-sts, default approle),
NBOX_GRPC_TLS (true|false; auto-on for :443, e.g. behind an ALB),
NBOX_LOG_LEVEL (default INFO; DEBUG shows the nbox.keepalive heartbeats).
"""

import base64
import json
import logging
import os
import re
import sys
import tempfile
import time
from collections.abc import Iterator
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Protocol

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.x25519 import X25519PrivateKey
from pyhpke import AEADId, CipherSuite, KDFId, KEMId, KEMKey

logging.basicConfig(
    level=os.environ.get("NBOX_LOG_LEVEL", "INFO").upper(),
    format="%(asctime)s.%(msecs)03d %(levelname)s %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger("watch")

_TS_RE = re.compile(r"ts=(\d+)")


def _lat_cols(value: object, server_ms: int) -> str:
    """Latency columns for load-test payloads (values carrying a `ts=` token):
      lat_ms = bus latency (now - server publish time `time_unix_ms`)
      e2e_ms = end-to-end  (now - client `ts`; includes the durable write)
    Returns '' for normal values, so non-load-test output is unchanged."""
    if not isinstance(value, str):
        return ""
    m = _TS_RE.search(value)
    if not m:
        return ""
    now = int(time.time() * 1000)
    cols = ""
    if isinstance(server_ms, int) and server_ms > 0:
        cols += f" lat_ms={now - server_ms}"
    cols += f" e2e_ms={now - int(m.group(1))}"
    return cols

# HPKE suite v1 (must match the server).
_KEM, _KDF, _AEAD = KEMId.DHKEM_X25519_HKDF_SHA256, KDFId.HKDF_SHA256, AEADId.AES256_GCM
_ENC_LEN = 32  # X25519 encapsulated-key length, prefixing the ciphertext.

# Reconnect backoff bounds (seconds).
MIN_BACKOFF, MAX_BACKOFF, BACKOFF_FACTOR = 1.0, 30.0, 2.0

# Server heartbeat type: keeps the ALB from idle-killing the stream. Skip it.
KEEPALIVE_TYPE = "nbox.keepalive"

# gRPC status codes that are terminal (do not retry).
_TERMINAL_CODES = frozenset({"UNAUTHENTICATED"})


@dataclass(frozen=True)
class AppConfig:
    grpc_target: str
    prefixes: list[str]
    auth_method: str
    tls: bool

    @classmethod
    def from_env(cls, args: list[str]) -> "AppConfig":
        target = os.environ.get("NBOX_GRPC", "localhost:9337")
        # TLS auto-detected for :443 (e.g. behind an ALB); override with
        # NBOX_GRPC_TLS=true|false.
        tls_env = os.environ.get("NBOX_GRPC_TLS")
        tls = tls_env.lower() == "true" if tls_env else target.endswith(":443")
        return cls(
            grpc_target=target,
            prefixes=args or ["passbox/"],
            auth_method=os.environ.get("NBOX_AUTH", "approle"),
            tls=tls,
        )


# --- Auth strategies: each produces the `authorization` metadata value ---

class Authorizer(Protocol):
    def header(self) -> str: ...


class AppRoleAuth:
    """AppRole credentials from env (NBOX_ROLE_ID / NBOX_SECRET_ID)."""

    def __init__(self, role_id: str, secret_id: str) -> None:
        self._role_id, self._secret_id = role_id, secret_id

    def header(self) -> str:
        cred = json.dumps({"role_id": self._role_id, "secret_id": self._secret_id}).encode("utf-8")
        return "AppRole " + base64.b64encode(cred).decode("ascii")


class AwsStsAuth:
    """Signs GetCallerIdentity (header-based sigv4) with the ambient AWS creds.

    The server replays the signed request to STS. Content-Type is set and
    signed so STS parses the form body and returns the GetCallerIdentity XML.
    Re-signed on each call (per reconnect) since the signature is time-bound.
    """

    def header(self) -> str:
        import boto3
        from botocore.auth import SigV4Auth
        from botocore.awsrequest import AWSRequest

        session = boto3.Session()
        creds = session.get_credentials().get_frozen_credentials()
        region = session.region_name or "us-east-1"

        req = AWSRequest(
            method="POST",
            url=f"https://sts.{region}.amazonaws.com/",
            data="Action=GetCallerIdentity&Version=2011-06-15",
            headers={"Content-Type": "application/x-www-form-urlencoded; charset=utf-8"},
        )
        SigV4Auth(creds, "sts", region).add_auth(req)

        body = req.body if isinstance(req.body, bytes) else req.body.encode("utf-8")
        wire = {
            "iam_http_request_method": req.method,
            "iam_request_url": base64.b64encode(req.url.encode("utf-8")).decode("ascii"),
            "iam_request_body": base64.b64encode(body).decode("ascii"),
            "iam_request_headers": base64.b64encode(json.dumps(dict(req.headers)).encode("utf-8")).decode("ascii"),
        }
        return "AWS-STS " + base64.b64encode(json.dumps(wire).encode("utf-8")).decode("ascii")


def build_authorizer(method: str) -> Authorizer:
    """Pick the auth strategy for NBOX_AUTH."""
    if method == "approle":
        try:
            return AppRoleAuth(os.environ["NBOX_ROLE_ID"], os.environ["NBOX_SECRET_ID"])
        except KeyError as e:
            raise ValueError(f"missing env var: {e}") from e
    if method == "aws-sts":
        return AwsStsAuth()
    raise ValueError(f"unknown NBOX_AUTH={method!r} (expected approle|aws-sts)")


class HpkeDecryptor:
    """Per-process X25519 keypair; opens HPKE-sealed vault values."""

    def __init__(self) -> None:
        self._priv = X25519PrivateKey.generate()
        self._skr = KEMKey.from_pyca_cryptography_key(self._priv)
        self._suite = CipherSuite.new(_KEM, _KDF, _AEAD)
        raw = self._priv.public_key().public_bytes(
            serialization.Encoding.Raw, serialization.PublicFormat.Raw
        )
        self.public_key_b64 = base64.b64encode(raw).decode("ascii")

    def decrypt(self, subject: str, data: bytes, is_hpke: bool) -> str:
        """Open a vault value; return non-vault data as-is."""
        if not is_hpke:
            return data.decode("utf-8", "replace")
        if len(data) < _ENC_LEN:
            raise ValueError("HPKE payload too short")
        enc, ct = data[:_ENC_LEN], data[_ENC_LEN:]
        info = f"nbox/vault/v1|{subject}".encode("utf-8")
        rctx = self._suite.create_recipient_context(enc, self._skr, info=info)
        return rctx.open(ct).decode("utf-8")


def decode_event(evt: Any, decryptor: HpkeDecryptor) -> tuple[str, str, str, int]:
    """Map a stream Event to (type, subject, value, server_publish_ms),
    decrypting vault payloads. `time_unix_ms` is the server publish time."""
    is_hpke = evt.extensions.get("encrypted") == "hpke"
    value = decryptor.decrypt(evt.subject, evt.data, is_hpke)
    return evt.type, evt.subject, value, evt.time_unix_ms


def env_name(subject: str) -> str:
    """Leaf key as an env-var-style name (passbox/av2/account-pepe -> ACCOUNT_PEPE)."""
    leaf = subject.rsplit("/", 1)[-1]
    return "".join(c if c.isalnum() else "_" for c in leaf).upper()


def next_backoff(current: float, *, factor: float = BACKOFF_FACTOR, cap: float = MAX_BACKOFF) -> float:
    """Exponential backoff capped at `cap`."""
    return min(current * factor, cap)


def is_terminal(code: Any) -> bool:
    """True when a gRPC status code means 'do not retry' (bad credentials)."""
    return getattr(code, "name", None) in _TERMINAL_CODES


def load_proto(proto_dir: Path) -> tuple[Any, Any, Any]:
    """Compile kvstream.proto into a temp dir and import the generated stubs."""
    proto_file = proto_dir / "kvstream.proto"
    if not proto_file.exists():
        raise FileNotFoundError(f"proto not found: {proto_file}")

    out = tempfile.mkdtemp(prefix="nbox_proto_")
    from grpc_tools import protoc
    if protoc.main([
        "protoc", f"-I{proto_dir}",
        f"--python_out={out}", f"--grpc_python_out={out}",
        str(proto_file),
    ]) != 0:
        raise RuntimeError("protoc failed to generate stubs")

    sys.path.insert(0, out)
    import grpc
    import kvstream_pb2 as pb
    import kvstream_pb2_grpc as pbg
    return grpc, pb, pbg


class NboxClient:
    """Handshake, stream, and reconnect loop. gRPC modules are injected."""

    def __init__(self, config: AppConfig, authorizer: Authorizer, decryptor: HpkeDecryptor,
                 grpc_mod: Any, pb: Any, pbg: Any) -> None:
        self.config = config
        self._authorizer = authorizer
        self._decryptor = decryptor
        self._grpc = grpc_mod
        self._pb = pb
        self._pbg = pbg

    def _metadata(self) -> list[tuple[str, str]]:
        return [
            ("authorization", self._authorizer.header()),
            ("x-vault-pubkey", self._decryptor.public_key_b64),
            ("x-vault-instance-nonce", "watch-py"),
        ]

    def _stream(self, stub: Any) -> Iterator[tuple[str, str, str, int]]:
        req = self._pb.WatchRequest(prefixes=self.config.prefixes)
        for evt in stub.Watch(req, metadata=self._metadata()):
            yield decode_event(evt, self._decryptor)

    def _channel(self) -> Any:
        # TLS for :443/ALB (system root CAs; ACM certs are publicly trusted),
        # plaintext h2c otherwise (local entrypushd).
        if self.config.tls:
            return self._grpc.secure_channel(
                self.config.grpc_target, self._grpc.ssl_channel_credentials()
            )
        return self._grpc.insecure_channel(self.config.grpc_target)

    def run(self) -> None:
        """Stream forever, reconnecting on transient failures.

        UNAUTHENTICATED is terminal (bad credentials) → exit. Any other drop
        (server restart, UNAVAILABLE) → reconnect with exponential backoff,
        reset once healthy traffic resumes.
        """
        backoff = MIN_BACKOFF
        while True:
            try:
                with self._channel() as ch:
                    stub = self._pbg.KVStreamStub(ch)
                    for evt_type, subject, value, server_ms in self._stream(stub):
                        backoff = MIN_BACKOFF  # healthy stream → reset (keepalives count)
                        if evt_type == KEEPALIVE_TYPE:
                            # Server heartbeat: entrypushd emits it on idle streams so the
                            # ALB's idle timeout doesn't kill the connection. No payload;
                            # visible with NBOX_LOG_LEVEL=DEBUG for local testing.
                            logger.debug("[%s] server heartbeat (keeps the stream alive behind the ALB)", evt_type)
                            continue
                        logger.info("[%s] %s = %r  (%s)%s", evt_type, subject, value, env_name(subject), _lat_cols(value, server_ms))
                logger.info("stream closed by server; reconnecting…")
            except self._grpc.RpcError as e:
                if is_terminal(e.code()):
                    logger.error("auth rejected (terminal): %s", e.details())
                    raise SystemExit(1) from e
                logger.warning("stream lost (%s); retry in %.0fs", e.code().name, backoff)
                time.sleep(backoff)
                backoff = next_backoff(backoff)
                continue
            time.sleep(MIN_BACKOFF)


def main() -> None:
    try:
        config = AppConfig.from_env(sys.argv[1:])
        authorizer = build_authorizer(config.auth_method)
    except ValueError as e:
        logger.error("config: %s", e)
        sys.exit(1)

    decryptor = HpkeDecryptor()
    try:
        proto_dir = Path(__file__).resolve().parents[3] / "proto"
        grpc, pb, pbg = load_proto(proto_dir)
    except (FileNotFoundError, RuntimeError) as e:
        logger.error("%s", e)
        sys.exit(1)

    logger.info("watching %s on %s (%s) via %s (Ctrl-C to stop)",
                config.prefixes, config.grpc_target,
                "tls" if config.tls else "plaintext", config.auth_method)
    try:
        NboxClient(config, authorizer, decryptor, grpc, pb, pbg).run()
    except KeyboardInterrupt:
        logger.info("disconnected.")


if __name__ == "__main__":
    main()
