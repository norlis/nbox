# NBOX Architecture Diagram

## Overview

NBOX is a backend platform for centralized management of configurations, environment variables, and secrets. It uses hexagonal architecture (ports and adapters) with native AWS integration, and adds a real-time change-streaming plane for agents plus a *vault* (passbox) mode where values are encrypted at rest and delivered end-to-end encrypted.

### Binaries

| Binary | Role | Listener |
|---|---|---|
| **nbox** (`cmd/nbox`) | Core HTTP REST API: entries, templates (box), prefix routing, auth (JWT). Wired with Uber `fx`. | HTTP `:7337` |
| **entrypushd** (`cmd/entrypushd`) | Real-time push daemon: subscribes to the NATS event bus and fans changes out to agents over a gRPC `Watch` stream. M2M auth (AppRole / AWS-STS). For `passbox/*` (vault) entries it delivers the value HPKE-sealed to the agent's key. | gRPC `:9337` |
| **nbox-cli** (`cmd/cli`, Cobra) | Admin CLI (`nbox-cli`): manages the dynamic-config table — prefix routing and auth entities (users, AppRole, AWS-STS). | — |

### External dependencies

- **AWS**: DynamoDB (entries index + metadata + tracking + dynamic config), S3 (templates), Parameter Store + KMS (secrets), STS (M2M identity & audit).
- **NATS**: event bus (subject `nbox.events.<env>`) connecting nbox (publisher) to every entrypushd instance (fan-out subscriber).

---

## General Architecture

```mermaid
flowchart TB
    subgraph CLIENTS["👥 Clients"]
        CLI["CLI/Scripts"]
        WEB["Web UI"]
        CICD["CI/CD"]
    end

    subgraph PRESENTATION["🌐 Presentation Layer"]
        AUTH["Authentication<br/>JWT + Basic Auth"]
        AUTHZ["Authorization<br/>OPA Policy"]
        API["HTTP API<br/>REST Endpoints"]
        SSE["Change Events<br/>→ NATS / entrypushd gRPC"]
    end

    subgraph APPLICATION["⚙️ Application Layer (Use Cases)"]
        ENTRY_UC["EntryUseCase<br/>Variables Management"]
        BOX_UC["BoxUseCase<br/>Templates Management"]
        PATH_UC["PathUseCase<br/>Keys Validation"]
        EVENT_UC["EventUseCase<br/>Notifications"]
        EXPORT_UC["ExportUseCase<br/>Data Export"]
    end

    subgraph DOMAIN["🏛️ Domain"]
        MODELS["Models<br/>Entry | Box | User<br/>Template | Event"]
        PORTS["Interfaces<br/>EntryAdapter<br/>TemplateAdapter<br/>SecretAdapter"]
    end

    subgraph INFRASTRUCTURE["🔌 Adapters"]
        DDB["DynamoDB<br/>Entries + Tracking"]
        S3["S3 Adapter<br/>Templates"]
        SSM["SSM Adapter<br/>Secrets"]
        MEMORY["InMemory<br/>Users"]
        SSE_BROKER["Event Publisher<br/>(NATS)"]
    end

    subgraph AWS["☁️ AWS Services"]
        DYNAMO[("DynamoDB")]
        BUCKET[("S3 Bucket")]
        PARAM[("Parameter Store")]
        KMS[("KMS")]
    end

    CLI --> AUTH
    WEB --> AUTH
    CICD --> AUTH

    AUTH --> AUTHZ
    AUTHZ --> API
    API --> SSE

    API --> ENTRY_UC
    API --> BOX_UC
    API --> EXPORT_UC

    ENTRY_UC --> PATH_UC
    ENTRY_UC --> EVENT_UC
    BOX_UC --> PATH_UC

    ENTRY_UC --> PORTS
    BOX_UC --> PORTS
    EVENT_UC --> PORTS
    EXPORT_UC --> PORTS

    PORTS --> MODELS

    DDB -.implements.-> PORTS
    S3 -.implements.-> PORTS
    SSM -.implements.-> PORTS
    MEMORY -.implements.-> PORTS
    SSE_BROKER -.implements.-> PORTS

    DDB --> DYNAMO
    S3 --> BUCKET
    SSM --> PARAM
    PARAM --> KMS

    SSE --> SSE_BROKER

    classDef clients fill:#E3F2FD,stroke:#1976D2,stroke-width:2px
    classDef presentation fill:#FFF3E0,stroke:#F57C00,stroke-width:2px
    classDef application fill:#E8F5E9,stroke:#388E3C,stroke-width:2px
    classDef domain fill:#FCE4EC,stroke:#C2185B,stroke-width:3px
    classDef infrastructure fill:#F3E5F5,stroke:#7B1FA2,stroke-width:2px
    classDef aws fill:#232F3E,stroke:#FF9900,stroke-width:3px,color:#fff

    class CLI,WEB,CICD clients
    class AUTH,AUTHZ,API,SSE presentation
    class ENTRY_UC,BOX_UC,PATH_UC,EVENT_UC,EXPORT_UC application
    class MODELS,PORTS domain
    class DDB,S3,SSM,MEMORY,SSE_BROKER infrastructure
    class DYNAMO,BUCKET,PARAM,KMS aws
```

---



## Authentication and Authorization Flow

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant Auth as Authn/Authz
    participant OPA
    participant UserRepo
    participant Handler

    Client->>API: Request + Credentials
    API->>Auth: Validate Auth

    alt Basic Auth
        Auth->>UserRepo: Check credentials
        UserRepo-->>Auth: User + Roles
    else JWT Token
        Auth->>Auth: Verify JWT signature
        Auth->>Auth: Extract user + roles
    end

    Auth->>OPA: Authorize (user, roles, resource, action)
    OPA-->>Auth: Allow/Deny

    alt Authorized
        Auth-->>API: User context
        API->>Handler: Process request
        Handler-->>Client: Response
    else Denied
        Auth-->>Client: 401/403 Error
    end
```

**Available Roles** (`policies/authz/roles.json`). Rego resolves the caller's roles into a union of permission patterns (regex over `METHOD:/path`); default-deny.

| Role | Description |
|---|---|
| `anonymous` | Public access only (health). |
| `authenticated` | Implicit pseudo-role injected by `policy.rego` for **every** authenticated caller (K8s `system:authenticated` style) — grants `platform:read:metadata`; never assigned to users. |
| `viewer` | Read-only non-production entries/templates + tracking. |
| `viewer_prod` | Read-only including production (key/prefix/export/lookup). |
| `editor` | Read+write entries and templates (+ boxspec validate). |
| `secrets_reader` | Read plaintext secret values (non-vault). |
| `maintainer` | Delete entries. |
| `cicd` | CI/CD: read templates+entries, build, CUE validate, bulk lookup, resolve non-vault secrets. |
| `entrypushd` | Service account for the entrypushd binary: read prefixes/leaves/lookup + `secrets:read:value:vault`. |
| `vault_reader` | Read vault (passbox) index + resolve its secret values + vault tracking — human counterpart of `entrypushd`. |
| `vault_operator` | `vault_reader` + `entries:write`. |
| `template_editor_nonprod` | Read all templates; create/update only non-prod stages; CUE validate. |
| `secrets_reader_nonprod` | Resolve plaintext for every root except beta/production/dr. |
| `templates_publisher` | Full template ops (read, write any stage, build, vars, validate). |
| `templates_publisher_nonprod` | Read all; write only non-prod; build/vars/validate. |
| `templates_viewer` | Read-only templates (list, detail, build, vars). |
| `entries_editor` | Create/update + read entries one level (no leaves/lookup). |
| `entries_reader` | Read all index records: key, prefix, lookup, export (no plaintext, no leaves). |
| `entries_reader_nonprod` | Read index records for development/qa/sandbox/global + history. |
| `entries_maintainer` | Delete entries (and subtree). |
| `security_auditor` | Read EVERYTHING incl. vault plaintext + history — zero writes. |
| `admin` | Full system access (`admin:full_access`). |

**M2M service accounts** authenticate with AppRole or AWS-STS (see *M2M Authentication* below) and receive these same roles inside a short-lived JWT.

---



## Variables Management Flow (Entries)

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant EntryUseCase
    participant SecretAdapter
    participant EntryAdapter
    participant EventUseCase
    participant DynamoDB
    participant SSM as Parameter Store
    participant SSE

    Note over Client,SSE: POST /api/entry (Upsert)

    Client->>API: Upsert entries<br/>[{key, value, secure}]
    API->>EntryUseCase: Upsert(entries)

    EntryUseCase->>EntryUseCase: Separate entries<br/>secure vs normal

    alt Entry is secure
        EntryUseCase->>SecretAdapter: Upsert secrets
        SecretAdapter->>SSM: PutParameter(encrypted)
        SSM-->>SecretAdapter: ARN
        SecretAdapter-->>EntryUseCase: Results with ARN
        EntryUseCase->>EntryUseCase: Replace value with ARN
    end

    EntryUseCase->>EntryAdapter: Upsert entries
    EntryAdapter->>DynamoDB: PutItem (entry)
    EntryAdapter->>DynamoDB: PutItem (tracking)
    DynamoDB-->>EntryAdapter: OK

    EntryAdapter-->>EntryUseCase: Results
    EntryUseCase->>EventUseCase: Emit event
    EventUseCase->>SSE: Broadcast update

    EntryUseCase-->>API: All results
    API-->>Client: Response

    SSE-->>Client: Real-time notification
```

**Available Operations:**
- `POST /api/entry`: Create/update variables
- `GET /api/entry/prefix?v=<path>[&leaves]`: List by prefix (one level; `leaves` = flat subtree of every real key)
- `GET /api/entry/key?v=<key>`: Get specific variable (index record)
- `GET /api/entry/resolve?v=<key>`: Resolve plaintext secret value
- `GET /api/entry/secret-value?v=<key>`: Deprecated alias of `/api/entry/resolve`
- `POST /api/entry/lookup`: Batch-resolve many keys (JSON body list)
- `GET /api/entry/export?prefix=<path>&format=<fmt>`: Export configuration
- `DELETE /api/entry/key?v=<key>`: Delete variable
- `GET /api/track/key?v=<key>&from=&to=&limit=`: Change history (time window)

---



## Templates Management Flow

```mermaid
sequenceDiagram
    participant Client
    participant API
    participant BoxUseCase
    participant S3Adapter
    participant DynamoDB
    participant S3

    Note over Client,S3: POST /api/box (Upsert Template)

    Client->>API: POST /api/box<br/>{service, stage, template}
    API->>BoxUseCase: Upsert template
    BoxUseCase->>BoxUseCase: Decode base64 content

    BoxUseCase->>S3Adapter: Store template
    S3Adapter->>S3: PutObject<br/>(service/stage/template.json)
    S3-->>S3Adapter: Version ID

    S3Adapter->>DynamoDB: Store metadata<br/>(version, timestamp)
    DynamoDB-->>S3Adapter: OK

    S3Adapter-->>BoxUseCase: Success
    BoxUseCase-->>API: Template stored
    API-->>Client: Response

    Note over Client,S3: GET /api/box/{service}/{stage}/{template}/build

    Client->>API: GET template + build<br/>?image-name=nginx:latest
    API->>BoxUseCase: Get and build template
    BoxUseCase->>S3Adapter: Retrieve template
    S3Adapter->>S3: GetObject
    S3-->>S3Adapter: Template content

    BoxUseCase->>BoxUseCase: Process template:<br/>1. Replace :placeholders<br/>2. Replace {{variables}}

    BoxUseCase->>DynamoDB: Retrieve variable values
    DynamoDB-->>BoxUseCase: Values

    BoxUseCase->>BoxUseCase: Build final template
    BoxUseCase-->>API: Processed template
    API-->>Client: Ready-to-use config
```

**Available Operations:**
- `POST /api/box/{service}/{stage}/{template}`: Create/update template (path-addressed, V2)
- `POST /api/box`: **Deprecated** form-body upsert (superseded by V2; admin-only)
- `GET /api/box`: List all templates (grouped and **ordered by service**)
- `GET /api/box/{service}/{stage}`: Stage detail (templates in a stage)
- `GET /api/box/{service}/{stage}/{template}`: Get raw template
- `HEAD /api/box/{service}/{stage}/{template}`: Check existence
- `GET /api/box/{service}/{stage}/{template}/build`: Get processed template
- `GET /api/box/{service}/{stage}/{template}/vars`: List variables a template references
- `GET /api/box/schemas`: List available schema types
- `GET /api/boxspec/specs` · `GET /api/boxspec/resolve?pattern=` · `POST /api/boxspec/validate` · `POST /api/boxspec/reload`: CUE spec catalog, JSON-Schema export, server-side validation, reload

**Template Processing:**
1. Placeholders `:variable` - Replaced with query params
2. Variables `{{global/example/var}}` - Replaced with values from DynamoDB/SSM

---



## Export Flow

```mermaid
flowchart LR
    Client["Client"]
    API["API Handler"]
    EXPORT["ExportUseCase"]
    ENTRY["EntryAdapter"]
    FORMAT["Format Exporter"]

    Client -->|"GET /api/export?prefix=dev&format=env"| API
    API --> EXPORT
    EXPORT -->|"List entries"| ENTRY
    ENTRY -->|"Entries"| EXPORT
    EXPORT -->|"Select exporter"| FORMAT

    subgraph EXPORTERS["Exporters"]
        ENV["ENV Exporter<br/>KEY=value"]
        JSON["JSON Exporter<br/>{key: value}"]
        YAML["YAML Exporter<br/>key: value"]
        DOCKER["Docker Compose<br/>environment"]
    end

    FORMAT --> EXPORTERS
    EXPORTERS -->|"Formatted output"| EXPORT
    EXPORT --> API
    API --> Client

    classDef client fill:#E3F2FD,stroke:#1976D2
    classDef usecase fill:#E8F5E9,stroke:#388E3C
    classDef exporter fill:#FFF3E0,stroke:#F57C00

    class Client client
    class EXPORT,ENTRY usecase
    class ENV,JSON,YAML,DOCKER exporter
```

**Supported Formats:**
- `env`: Environment variables (.env)
- `json`: JSON format
- `yaml`: YAML format
- `docker-compose`: For docker-compose.yml

---





## Real-time Change Streaming (NATS → entrypushd → gRPC)

Real-time delivery is **not** SSE/HTTP. nbox publishes change events to a **NATS** bus; **every** `entrypushd` instance subscribes (empty queue group ⇒ fan-out — all instances get all events) and forwards matching events to agents over the gRPC `stream.v1.KVStream/Watch` server-stream.

```mermaid
flowchart LR
    subgraph NBOX["nbox (:7337)"]
        REC["tracking.Recorder<br/>(decorates entry.Manager)"]
        PUB["CloudEvent Publisher"]
    end

    NATS[("NATS bus<br/>nbox.events.&lt;env&gt;")]

    subgraph EP["entrypushd (:9337)"]
        SUB["NATS subscriber<br/>(fan-out)"]
        BROKER["in-memory Broker<br/>(cap 20, drop-if-full)"]
        VAULT["HPKE seal<br/>(passbox/*)"]
    end

    subgraph AGENTS["Agents (gRPC clients)"]
        A1["Watch(prefixes, types)"]
        A2["Watch + x-vault-pubkey"]
    end

    REC --> PUB --> NATS --> SUB --> BROKER
    BROKER --> A1
    BROKER --> VAULT --> A2

    classDef nbox fill:#FFF3E0,stroke:#F57C00
    classDef bus fill:#232F3E,stroke:#FF9900,color:#fff
    classDef ep fill:#E8F5E9,stroke:#388E3C
    classDef agent fill:#F3E5F5,stroke:#7B1FA2
    class REC,PUB nbox
    class NATS bus
    class SUB,BROKER,VAULT ep
    class A1,A2 agent
```

**Event flow:**
- Event types (CloudEvents): `nbox.entry.upserted`, `nbox.entry.deleted` (entrypushd forwards these two). Subject = the entry key; extension `prefix` = top path segment.
- `Watch(prefixes[], types[])` filter: OR within each list, AND across the two; `*`-suffix wildcard on types; empty list = match-all. Per-subscriber buffered channel (cap 20), drop-if-full so a slow agent never blocks the bus.
- **Snapshot-on-connect** (`ENTRYPUSHD_NBOX_URL` set + prefixes given): before streaming deltas, entrypushd fetches current state via `GET /api/entry/prefix?v=<pfx>&leaves=true` and replays it as `EntryUpserted` bursts — no gap, subscription registered first. Fail-closed (`Unavailable`) on fetch error.

---

## M2M Authentication (entrypushd)

Agents authenticate to the gRPC stream via one of two Vault-style M2M schemes; both mint the **same** short-lived JWT (TTL **15 min**, `aud=["nbox","entrypushd"]`, signed with the shared `HMAC_SECRET_KEY`).

- **AppRole** — `authorization: AppRole <base64(JSON{role_id, secret_id})>`. bcrypt-matched against the config store; supports per-hash `ExpiresAt`, `allowed_cidrs`, zero-downtime secret rotation.
- **AWS-STS** — `authorization: AWS-STS <base64(JSON{presigned GetCallerIdentity})>`. entrypushd forwards it to STS (anti-SSRF host allowlist), matches the returned ARN against `arn_map`.

**Hardening:** kill switch `NBOX_APPROLE_DISABLED=true` (→ `Unavailable`); CIDR restrictions; **sentinel collapse** (all credential failures → `Unauthenticated "invalid credentials"`, no oracle); audit logs record `role_id`/scheme/source_ip/rpc_method/success — never the `secret_id`.

---

## Vault (passbox) — encrypted at rest & in transit

A prefix is *vault* through three real mechanisms (there is no `isVault` field):
1. **prefix-config** routes `passbox` writes to `parameterstore_secure` → forces `secure=true` (KMS-encrypted), never stored as plain text.
2. **entrypushd** hardcodes `vaultPrefix = "passbox/"` (`isVaultKey`).
3. **OPA** gates it with `*:vault` permissions matching `v=passbox…`.

**Key format:** `/passbox/[stage/]<service>/<secret>`. Required tags `stage_name`, `updated_by`; max 10 tags.

**HPKE delivery (RFC 9180, X25519/HKDF-SHA256/AES-256-GCM):** when a `Watch` agent presents `x-vault-pubkey` + `x-vault-instance-nonce`, entrypushd resolves the plaintext (`GET /api/entry/resolve`) and **seals it to the agent's ephemeral key** instead of masking — ciphertext in `Event.Data`, `Extensions{encrypted:hpke, suite_id, key_fpr}`. Plaintext exists only transiently in entrypushd RAM; never logged or persisted. Fail-closed. A non-forgeable `client_id = H(subject‖nonce‖pubkey_fpr)` is derived for audit.

---





## Data Storage

```mermaid
erDiagram
    ENTRIES_TABLE ||--o{ TRACKING_TABLE : tracks
    ENTRIES_TABLE {
        string partition_key
        string value
        bool secure
        string createdAt
        string updatedAt
    }

    TRACKING_TABLE {
        string partition_key
        string sort_key
        string oldValue
        string newValue
        string user
        string action
    }

    BOX_TABLE ||--o{ S3_TEMPLATES : references
    BOX_TABLE {
        string partition_key
        string sort_key
        string s3Key
        string version
        string createdAt
        string updatedAt
    }

    S3_TEMPLATES {
        string key
        string content
        string versionId
    }

    PARAMETER_STORE {
        string name
        string value
        string kmsKeyId
        string type
    }

    ENTRIES_TABLE ||--o{ PARAMETER_STORE : secure_entries_reference
```

---





## Use Cases

### Application Deployment with ECS

```mermaid
sequenceDiagram
    participant DEV as Developer
    participant NBOX
    participant CI as CI/CD Pipeline
    participant ECS
    participant APP as Application

    DEV->>NBOX: 1. Create variables<br/>POST /api/entry<br/>[DB_HOST, API_KEY]
    DEV->>NBOX: 2. Upload task definition template<br/>POST /api/box

    Note over DEV,NBOX: Template contains:<br/>{{dev/myapp/DB_HOST}}<br/>{{dev/myapp/API_KEY}}

    CI->>NBOX: 3. Build template<br/>GET /api/box/myapp/dev/task.json/build
    NBOX-->>CI: Processed task definition

    CI->>ECS: 4. Register task definition
    CI->>ECS: 5. Deploy service

    ECS->>APP: 6. Start containers with<br/>variables and secrets
```



### Secrets Rotation

```mermaid
sequenceDiagram
    participant ADMIN as Admin
    participant NBOX
    participant SSM as Parameter Store
    participant SSE
    participant APPS as Running Apps

    ADMIN->>NBOX: 1. Update secret<br/>POST /api/entry<br/>[{key: "prod/db/password", value: "new-pass", secure: true}]

    NBOX->>SSM: 2. Store encrypted value
    SSM-->>NBOX: ARN

    NBOX->>NBOX: 3. Store ARN reference in DynamoDB
    NBOX->>SSE: 4. Emit event

    SSE-->>APPS: 5. Notify: "prod/db/password updated"

    Note over APPS: Apps can:<br/>- Reload config<br/>- Restart<br/>- Notify status
```

---



## Storage Backends & Prefix Routing

Each entry is routed to a storage backend by **longest-prefix-match (LPM)** over the prefix-config, combined with the entry's `secure` flag (`internal/entry/store/gateway.go`).

- Backends: `dynamodb` (also the global **index**), `parameterstore`, `parameterstore_secure` (forces KMS).
- Per-prefix `Config` has `TypeDefault` and `TypeSecure`; `resolveBackend(key, secure)` picks `TypeSecure` when the entry is secure, else `TypeDefault`. LPM is an immutable in-memory map rebuilt on each config refresh — no DynamoDB round-trip per resolve.
- The DynamoDB **index** always holds the metadata record (stamped with `StorageBackend` + SHA-256 `Fingerprint`); the real value lives in the routed backend. `Resolve` reads the index then fetches the value from the named backend. For secure entries the index value is a pointer/ARN, never plaintext.
- Inspect routing at runtime: `GET /api/prefix` (list), `GET /api/prefix/backends`, `GET /api/prefix/resolve?v=<key>` (LPM resolution).

## Dynamic Configuration (hot-reload)

`NBOX_BASIC_AUTH_CREDENTIALS`, `NBOX_APPROLE_ROLES`, `NBOX_AWS_ARN_MAP` and prefix routing resolve via an **env-first, then-DynamoDB** chain (`internal/config/`). One config table (`NBOX_CONFIG_TABLE_NAME`, key `kind`+`id`) holds four kinds:

| kind | Managed by | Purpose |
|---|---|---|
| `basic_auth` | `nbox-cli config user` | HTTP Basic Auth users (bcrypt) |
| `app_role` | `nbox-cli config approle` | AppRole M2M credentials |
| `arn_map` | `nbox-cli config aws-sts` | AWS-STS ARN → roles |
| `prefix_config` | `nbox-cli prefix` | storage routing per prefix |

If the env var is set it **wins** and is static (restart to change). The DynamoDB path uses a TTL cache (`NBOX_CONFIG_TTL`, default 45s) with background refresh, atomic swap, and **fail-closed** startup. Changes propagate to all replicas within one TTL; `NBOX_APPROLE_DISABLED` is the immediate revocation kill switch. Every write stamps `updated_by` with the caller's AWS ARN (`sts:GetCallerIdentity`).

## Admin CLI (`nbox-cli`)

Cobra CLI that writes the dynamic-config table without restarts. Two command groups:

- **`prefix`** — `upsert` / `list` / `rm` storage-routing configs (`--prefix`, `--type`, `--secure`, `--allowed`).
- **`config`** — three subgroups, each `upsert`/`list`/`rm` (+ approle `generate`/`rotate-secret`):
  - `config user` — basic-auth users (`--username`, `--password` → bcrypt, `--roles`, `--status`).
  - `config aws-sts` — ARN mappings (`--arn`, `--roles`).
  - `config approle` — `generate` (mints `role_id`+`secret_id`), `rotate-secret` (appends a new hash, zero-downtime), `--cidrs`.
- **`--emit-env`** (on every write command): print the `export NBOX_XXX=…` line for local dev instead of touching DynamoDB.

---

## Key Components

### Configuration (Config)
- Loaded from environment variables
- Support for multiple credential strategies
- Configurable environment prefixes

### Path Use Case
- Key validation
- Path normalization
- Allowed prefixes control

### Event System
- Decorator pattern for use cases
- Multi-channel publishing
- SSE support for Web UI

### Health Checks
- AWS connectivity verification
- S3, DynamoDB, SSM status
- Endpoints `/health`, `/status`, `/ready` (public whitelist)

---



## Security

### Security Layers

1. **Authentication**: HTTP Basic Auth or JWT
2. **Authorization**: OPA (Open Policy Agent) with role-based policies. Platform metadata endpoints (stages, `me/permissions`, prefix, schemas, boxspec) are no longer in the public whitelist — they require authentication and are granted via the implicit `authenticated` pseudo-role (`platform:read:metadata`). The public whitelist keeps only health checks, `POST /api/auth/token`, and Swagger.
3. **Encryption**:
   - Secrets encrypted in Parameter Store with KMS
   - TLS in transit (deployment configuration)
4. **Auditing**: Change tracking in DynamoDB table

### OPA Authorization Flow

```mermaid
flowchart LR
    REQUEST["Request"] --> EXTRACT["Extract<br/>user, roles, resource, action"]
    EXTRACT --> OPA["OPA Engine"]
    OPA --> POLICY["policy.rego"]
    OPA --> ROLES["roles.json"]
    OPA --> PERMS["permissions.json"]

    POLICY --> DECISION{Allow?}
    ROLES --> DECISION
    PERMS --> DECISION

    DECISION -->|Yes| ALLOW["Process Request"]
    DECISION -->|No| DENY["403 Forbidden"]

    classDef process fill:#E3F2FD,stroke:#1976D2
    classDef decision fill:#FFF3E0,stroke:#F57C00
    classDef result fill:#C8E6C9,stroke:#2E7D32
    classDef error fill:#FFCDD2,stroke:#C62828

    class REQUEST,EXTRACT,OPA,POLICY,ROLES,PERMS process
    class DECISION decision
    class ALLOW result
    class DENY error
```

---

## Endpoints Summary

### Public (no auth — whitelist)
- `GET /health` · `GET /status` · `GET /ready` - Health / liveness / readiness
- `POST /api/auth/token` - Basic Auth → JWT (24h)
- `GET|POST /swagger/` - Swagger UI

### Platform metadata (requires auth via `authenticated`)
- `GET /api/static/stages` - Valid stages
- `GET /api/me/permissions` - Caller's granted permissions (UI hints)
- `GET /api/prefix` · `GET /api/prefix/backends` · `GET /api/prefix/resolve?v=<key>` - Prefix routing

### Entries (Variables)
- `POST /api/entry` - Create/update variables
- `GET /api/entry/prefix?v=<path>[&leaves]` - List by prefix (one level / flat subtree)
- `GET /api/entry/key?v=<key>` - Get variable (index record)
- `GET /api/entry/resolve?v=<key>` - Resolve secret (plaintext)
- `GET /api/entry/secret-value?v=<key>` - Deprecated alias of `/api/entry/resolve`
- `POST /api/entry/lookup` - Batch-resolve many keys
- `GET /api/entry/export?prefix=<path>&format=<fmt>` - Export configuration
- `DELETE /api/entry/key?v=<key>` - Delete variable
- `GET /api/track/key?v=<key>&from=&to=&limit=` - Change history

### Box (Templates) & Specs
- `POST /api/box/{service}/{stage}/{template}` - Create/update template (V2)
- `POST /api/box` - Deprecated form-body upsert (admin-only)
- `GET /api/box` - List all templates (ordered by service)
- `GET /api/box/{service}/{stage}` - Stage detail
- `GET|HEAD /api/box/{service}/{stage}/{template}` - Get / check template
- `GET /api/box/{service}/{stage}/{template}/build` - Processed template
- `GET /api/box/{service}/{stage}/{template}/vars` - Template variables
- `GET /api/box/schemas` - Schema types
- `GET /api/boxspec/specs` · `GET /api/boxspec/resolve?pattern=` · `POST /api/boxspec/validate` · `POST /api/boxspec/reload` - CUE specs

### gRPC (entrypushd, :9337)
- `stream.v1.KVStream/Watch` - Server-streaming change subscription. Requires `authorization: AppRole|AWS-STS <base64…>` metadata. Vault values HPKE-sealed when `x-vault-pubkey` is presented.

---



## Technologies Used

- **Language**: Go 1.26
- **DI**: Uber FX
- **HTTP**: Native net/http (Go 1.22 method-prefixed ServeMux)
- **gRPC**: `stream.v1.KVStream` (entrypushd) + reflection
- **Event bus**: NATS (core, fan-out)
- **AWS SDK**: aws-sdk-go-v2 (DynamoDB, S3, SSM/Parameter Store, KMS, STS)
- **AuthN**: JWT (HS256) for humans; AppRole & AWS-STS M2M for agents; bcrypt password/secret hashing
- **AuthZ**: OPA (Open Policy Agent) — Rego policies
- **Crypto**: HPKE (RFC 9180, X25519/HKDF-SHA256/AES-256-GCM) for vault delivery
- **Templates**: CUE spec validation
- **Logger**: Zap (structured logging)
- **CLI**: Cobra
- **Swagger**: OpenAPI documentation
- **Testing**: Go standard testing

