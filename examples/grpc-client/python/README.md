# Python client

`watch.py` subscribes to a prefix and decrypts vault (`passbox/*`) values
(HPKE), reconnecting on stream loss. See [../README.md](../README.md) for the
wire contract.

Dependencies are declared inline in the script (PEP 723) — `uv` installs
them on first run. Needs Python 3.14 (uv fetches it automatically).

Auth method is chosen with `NBOX_AUTH` (`approle` | `aws-sts`, default `approle`).

Common env: `NBOX_GRPC` (default `localhost:9337`).

## AppRole (default)

Credentials in env vars.

```bash
export NBOX_ROLE_ID=...  NBOX_SECRET_ID=...
uv run watch.py                    # default prefix: passbox/
uv run watch.py development/ qa/   # custom prefixes
```

## AWS-STS (agents inside AWS)

For an agent inside AWS (EC2 / ECS / EKS / Lambda), drop `role_id`/`secret_id`
and use the workload's own IAM credentials: `watch.py` sigv4-signs a
`GetCallerIdentity` request; entrypushd forwards it to STS and matches the ARN
against `NBOX_AWS_ARN_MAP`. See [../README.md](../README.md#aws-sts-scheme) and
[../AWSSTS-TESTING.md](../AWSSTS-TESTING.md) for the server side.

```bash
# aws sts get-caller-identity 2>&1 | head -20
export NBOX_AUTH=aws-sts
uv run watch.py
```

No nbox secrets needed — the AWS credentials come from the environment (env
vars, instance profile, IRSA, etc.). The credential is re-signed on each
reconnect so it never goes stale.
