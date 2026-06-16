# Probar AWS-STS auth end-to-end

Validar el flujo `agente → gRPC → entrypushd → AWS STS` con credenciales AWS reales.

## Cómo funciona

El agente firma un `GetCallerIdentity` con sus credenciales AWS y lo manda en la
metadata gRPC. entrypushd lo reenvía a STS, obtiene el ARN de quien firmó, lo
busca en `NBOX_AWS_ARN_MAP` y, si matchea, mintea un JWT interno para el stream.
entrypushd nunca ve las credenciales AWS del agente.

## 1. Identificá tu ARN

Usá cualquier identidad AWS que tengas (SSO, instance profile, user):

```bash
aws sts get-caller-identity
```

entrypushd **normaliza** el ARN antes de matchear: un
`assumed-role/<rol>/<sesión>` se convierte en `arn:aws:iam::<cuenta>:role/<rol>`
(sin la sesión). Ese rol normalizado es el que va en el map.

## 2. Configurá `NBOX_AWS_ARN_MAP` y reiniciá entrypushd

```bash
NBOX_AWS_ARN_MAP='[{"arn":"arn:aws:iam::123456789012:role/mi-rol","name":"local","roles":["entrypushd"],"status":"active"}]'
```

- El match es **exacto** (byte a byte tras normalizar).
- **SSO / IAM Identity Center:** los roles se llaman
  `AWSReservedSSO_<PermissionSet>_<hash>` (ej.
  `AWSReservedSSO_ReadOnlyAccess_48a7f4deb41484bf`). El hash **es parte del
  nombre** — copialo tal cual de `get-caller-identity`.
- entrypushd lee el map **solo al arrancar**: reinicialo después de editarlo.

## 3. Conectá un cliente

**Cliente Python** (lo más cercano a un agente real):

```bash
export NBOX_AUTH=aws-sts
uv run examples/grpc-client/python/watch.py
```

Toma las credenciales del entorno, firma con SigV4 header-based y re-firma en
cada reconexión (la ventana sigv4 dura 15 min). No necesita secretos de nbox.

**Alternativa con grpcurl:** generá la credencial con el helper
`awssts.BuildCredential` (`internal/auth/awssts/client.go`) y pasala como
metadata:

```bash
grpcurl -plaintext -H "authorization: AWS-STS <base64>" -d '{}' \
  localhost:9337 stream.v1.KVStream/Watch
```

## 4. Verificá

En el log de entrypushd:

```
{"msg":"auth success","scheme":"AWS-STS","success":true}
```

El cliente empieza a recibir el snapshot y los eventos.

## Troubleshooting

El cliente **siempre** recibe `Unauthenticated: invalid credentials` (colapso
anti-oracle — no filtra qué check falló). El motivo real está en el audit log de
entrypushd, en `error_kind` y `error` (este último trae el `Code: Message`
exacto que devolvió STS):

| `error_kind` | Causa | Dónde se arregla |
|---|---|---|
| `unknown_arn` | STS validó OK pero el ARN no está en `NBOX_AWS_ARN_MAP` | server: agregar/corregir el ARN |
| `arn_disabled` | el ARN está en el map pero `status != active` | server: `"status":"active"` |
| `sts_rejected` | STS rechazó la firma (ver `error`) | cliente (abajo) |
| `untrusted_host` | el host STS del wire no está en la whitelist anti-SSRF | revisar `AWS_ENDPOINT_URL_STS` |
| `sts_unavailable` | STS caído / problema de red (5xx, timeout) | reintentar |

**`unknown_arn`** — confirmá tu ARN con `get-caller-identity`, aplicá la
normalización y comparalo byte a byte con el map (ojo con el hash de SSO).
Reiniciá entrypushd si editaste el map.

**`sts_rejected`** — el `error` dice por qué:

- `SignatureDoesNotMatch`: la firma no valida. En Python pasa si cambiás a
  presigned-URL (`generate_presigned_url`); quedate con header-based (default de
  `watch.py`).
- `parse xml`: STS respondió 200 pero sin el XML esperado, casi siempre por
  falta del `Content-Type` firmado (ya incluido en `watch.py`). El `error` trae
  un snippet del body.
- `InvalidClientTokenId` / `ExpiredToken`: credenciales AWS inválidas o vencidas;
  renovalas (re-login SSO; instance profile / IRSA refrescan solos).
