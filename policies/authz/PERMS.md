# Authz: permisos y roles

> Snapshot of `permissions.json` + `roles.json` — update this file whenever those change.

## Permisos (27)

| Permiso | Patterns | Descripción |
|---|---|---|
| `admin:full_access` | `(GET\|POST\|PUT\|DELETE\|HEAD):/.*` | Full access to all resources |
| `public:health` | `^GET:/health$` · `^GET:/test$` | Health check endpoints |
| `platform:read:metadata` | `^GET:/api/static/stages$` · `^GET:/api/me/permissions$` · `^GET:/api/prefix$` · `^GET:/api/prefix/backends$` · `^GET:/api/prefix/resolve\?v=(.*)` · `^GET:/api/box/schemas$` · `^GET:/api/boxspec/specs$` · `^GET:/api/boxspec/resolve\?(.*)` | Platform metadata for every authenticated caller (via the implicit `authenticated` pseudo-role) |
| `templates:read` | `^GET:/api/box$` · `^GET:/api/box/[^/]+/[^/]+$` · `^GET:/api/box/(.*)/(.*)/(.*)$` · `^HEAD:/api/box/(.*)/(.*)/(.*)` | Read all templates (list, stage detail, template, HEAD) |
| `templates:read:non_production` | `^GET:/api/box/(.*)/(qa\|development)/[^/]+\.json$` | Read templates only from QA and development |
| `templates:read:build` | `^GET:/api/box/(.*)/(.*)/(.*)/build$` | View template build output |
| `templates:read:vars` | `^GET:/api/box/(.*)/(.*)/(.*)/vars$` | View template variables |
| `templates:write` | `^POST:/api/box/[^/]+/[^/]+/[^/]+$` | Create/update a template via the path-addressed route (the body-based POST /api/box is deprecated and no longer granted) |
| `templates:write:non_production` | `^POST:/api/box/[^/]+/(development\|qa\|beta\|sandbox)/[^/]+$` | Create/update templates only in non-production stages (path-scoped) |
| `boxspec:validate` | `^POST:/api/boxspec/validate$` | Validate content against the CUE specs (server-side evaluation) |
| `boxspec:admin` | `^POST:/api/boxspec/reload$` | Reload CUE spec definitions from disk (mutates process state) |
| `entries:read:key` | `^GET:/api/entry/key\?v=(.*)` | Read individual entry by key |
| `entries:read:key:non_production` | `^GET:/api/entry/key\?v=(development\|qa\|global)/(.*)` | Read entries only from development, QA, and global |
| `entries:read:key:vault` | `^GET:/api/entry/key\?v=passbox(/\|%2[Ff])(.*)` | Read a single vault (passbox) index record |
| `entries:read:prefix` | `^GET:/api/entry/prefix\?v=[^&]*$` | List one level of entries by prefix (v as the only query param — the leaves mode requires entries:read:prefix:leaves) |
| `entries:read:prefix:non_production` | `^GET:/api/entry/prefix\?v=(development\|qa\|global)/[^&]*$` | List one level only for development/qa/global (no leaves mode) |
| `entries:read:prefix:vault` | `^GET:/api/entry/prefix\?(.*&)?v=passbox(/\|%2[Ff])?(.*)` | List vault (passbox) index records by prefix |
| `entries:read:prefix:leaves` | `^GET:/api/entry/prefix\?(.*&)?leaves(=[^&]*)?(&.*)?$` | Flat-subtree listing (leaves mode) of entries by prefix — separate grant because it returns the whole subtree in one call |
| `entries:read:lookup` | `^POST:/api/entry/lookup$` | Bulk read of entry index records by key list (body-based: cannot be prefix-scoped at route level — grant only to roles that may read every prefix) |
| `entries:read:export` | `^GET:/api/entry/export\?(.*)` | Export entries |
| `entries:write` | `^POST:/api/entry$` | Create/update entries |
| `entries:delete` | `^DELETE:/api/entry/key\?v=(.*)` | Delete entries |
| `secrets:read:value` | `^GET:/api/entry/resolve\?v=(development\|qa\|beta\|sandbox\|production\|dr\|global)(/\|%2[Ff])(.*)` | Resolve plaintext secret values for NON-vault keys (passbox excluded by root enumeration: RE2 has no negative lookahead) |
| `secrets:read:value:non_production` | `^GET:/api/entry/resolve\?v=(development\|qa\|sandbox\|global)(/\|%2[Ff])(.*)` | Resolve plaintext secret values for every root except beta/production/dr |
| `secrets:read:value:vault` | `^GET:/api/entry/resolve\?v=passbox(/\|%2[Ff])(.*)` | Resolve plaintext ONLY for vault (passbox) keys. Accepts the '/' literal and its %2F url-encoded form (the entrypushd client encodes slashes). |
| `tracking:read` | `^GET:/api/track/key\?v=(.*)` | View entry history |
| `tracking:read:vault` | `^GET:/api/track/key\?(.*&)?v=passbox(/\|%2[Ff])(.*)` | Change history of vault (passbox) keys |

## Roles (22)

| Rol | Permisos |
|---|---|
| `anonymous` | `public:health` |
| `authenticated` | `platform:read:metadata` (pseudo-rol implícito — inyectado por policy.rego, nunca se asigna a usuarios) |
| `viewer` | `templates:read:non_production`, `templates:read`, `entries:read:key:non_production`, `entries:read:prefix:non_production`, `tracking:read` |
| `viewer_prod` | `entries:read:key`, `entries:read:prefix`, `entries:read:export`, `entries:read:lookup` |
| `editor` | `templates:read`, `templates:write`, `templates:read:build`, `templates:read:vars`, `entries:write`, `entries:read:key`, `entries:read:prefix`, `entries:read:lookup`, `boxspec:validate` |
| `secrets_reader` | `secrets:read:value` |
| `maintainer` | `entries:delete` |
| `cicd` | `templates:read`, `templates:read:build`, `entries:read:key`, `entries:read:prefix`, `entries:read:lookup`, `boxspec:validate` |
| `entrypushd` | `entries:read:prefix`, `entries:read:prefix:leaves`, `secrets:read:value:vault` |
| `vault_reader` | `entries:read:prefix:vault`, `entries:read:key:vault`, `secrets:read:value:vault`, `tracking:read:vault` |
| `vault_operator` | `entries:read:prefix:vault`, `entries:read:key:vault`, `secrets:read:value:vault`, `tracking:read:vault`, `entries:write` |
| `template_editor_nonprod` | `templates:read`, `templates:write:non_production`, `templates:read:build`, `templates:read:vars`, `boxspec:validate` |
| `secrets_reader_nonprod` | `secrets:read:value:non_production` |
| `admin` | `admin:full_access` |
