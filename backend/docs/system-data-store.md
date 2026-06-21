# System Data Store (`_system`)

Status: **Design / planned** — not yet implemented.
Last updated: 2026-06-21.

## 1. Purpose

ProtoCMS needs a place to persist **system-related data** that is not tenant
content: user accounts and API keys (and, later, other operational records).
Today these credentials come **only** from environment variables
(`PROTOCMS_API_KEYS`, `PROTOCMS_USERS`, `PROTOCMS_JWT_SECRET`), which is fine for
local testing but cannot be managed at runtime.

This document specifies a dedicated dataset, `_system`, that becomes the
**source of truth** for credentials, with the existing env-var config retained
as a debug/testing **override** layer.

## 2. Goals & non-goals

**Goals**
- A persistent store named `_system`, always loaded at startup.
- Stores **users** and **API keys**, each carrying the **dataset** they grant
  access to and a **role** (`admin` | `editor`).
- API keys stored **hashed** at rest; plaintext shown once at creation.
- Env-var credentials keep working as a **debug override** that shadows
  `_system`.
- `_system` is **internal-only**: never reachable through the tenant content API.

**Non-goals (future work)**
- Live config hot-reload from disk (file watching).
- Key rotation policies, password reset flows, audit logging.
- Moving the JWT secret into `_system` (stays env-only for now).

## 3. Why this fits the current architecture

The store already supports **multiple concurrent datasets** via a `Registry`
(`backend/store/registry.go`), each with its own `sync.RWMutex` and its own
on-disk folder. `main.go` already preloads datasets referenced by credentials at
startup. So `_system` is just **another dataset** — no changes to the core store
engine are required.

> Note: `CLAUDE.md` currently describes persistence as "a single JSON file per
> dataset under `data/<dataset>.json`". The actual on-disk format is **v2
> folder-based**: `data/<name>/{data.json,meta.json}`. That doc line should be
> corrected as part of this work.

## 4. On-disk layout

`_system` is a normal v2 dataset:

```
data/_system/
  data.json   # schemas + content (collections: "users", "apikeys")
  meta.json   # dataset metadata (format version, timestamps)
```

Two logical collections live inside `data.json`'s `content` map:

### `users`
| Field          | Type   | Notes                                  |
|----------------|--------|----------------------------------------|
| `id`           | int    | store-assigned                         |
| `username`     | string | unique                                 |
| `password_hash`| string | salted hash (see §6)                   |
| `role`         | string | `admin` \| `editor`                    |
| `dataset`      | string | dataset this user is bound to          |
| `status`       | string | e.g. `active` \| `disabled`            |
| `created_at`   | string | RFC3339 UTC                            |
| `updated_at`   | string | RFC3339 UTC                            |

### `apikeys`
| Field          | Type   | Notes                                       |
|----------------|--------|---------------------------------------------|
| `id`           | int    | store-assigned                              |
| `name`         | string | human label                                 |
| `prefix`       | string | first ~8 chars of the key, for UI display   |
| `hash`         | string | salted hash of the full key (see §6)        |
| `role`         | string | `admin` \| `editor`                         |
| `dataset`      | string | **dataset this key serves data from**       |
| `created_at`   | string | RFC3339 UTC                                  |
| `last_used_at` | string | RFC3339 UTC, optional                       |
| `revoked_at`   | string | RFC3339 UTC; set ⇒ excluded from auth        |

The `dataset` field on an API key is what `datasetForRequest` uses to serve data
from that dataset — exactly the role the optional `:dataset` env segment plays
today.

## 5. Credential model: env overrides `_system`

`_system` is the **primary** credential store. Env vars are a **debug/testing
override** that shadows it.

- **Precedence (a):** on lookup, if a token/user exists in **env**, env's
  role/dataset wins. Otherwise fall back to `_system`.
- **Timing:** a single merged credential map is built at startup (env layered
  over `_system`), and **re-merged on every `_system` credential write** so
  changes made through the management API take effect immediately in-process
  (no restart). Disk is not watched; external edits to `data/_system/` still
  require a restart.

### Verification flow
1. **Startup:** parse env (`PROTOCMS_API_KEYS`, `PROTOCMS_USERS`) → load
   `_system` users/keys → merge with **env-over-`_system`** precedence into:
   - `map[token] → {role, dataset}` for API keys (revoked keys excluded)
   - `map[username] → {passwordHash, role, dataset}` for users
2. **`NewVerifier`** reads the merged maps. Call-site behavior is unchanged — it
   still returns `middleware.Claims`.
3. **On `_system` credential write** (create/revoke key, add/update user) the
   merge is re-run, refreshing the in-process maps.
4. **`/api/login`** validates against the merged user map (env password entries
   still win, for debugging).

### Bootstrap / lockout safety
Because `_system` is authoritative, an empty `_system` with no env credentials
would mean no way in. Guard at startup: if there is **no admin** in either env or
`_system`, log a loud warning and rely on the env override as the always-present
escape hatch. Env-overrides-`_system` makes this natural — set an env admin key
and you are guaranteed access regardless of `_system` contents.

## 6. API key & password hashing

- Keys are high-entropy random strings; storage uses a **salted SHA-256** hash
  with a per-record random salt, compared in constant time. SHA-256 is
  sufficient for high-entropy secrets and keeps the project dependency-free
  (stdlib only, consistent with the hand-rolled JWT in `auth.go`).
- On creation the **plaintext key is returned once** and never stored; only
  `hash` + `prefix` persist.
- User passwords are likewise stored hashed (salted). If a stronger KDF is
  wanted later (bcrypt/argon2), that pulls in `golang.org/x/crypto` — deferred.

## 7. Isolation: internal-only

`_system` must never be reachable through the tenant content API.

- **Reserve the `_` prefix.** In `auth` dataset resolution and in
  `datasetForRequest` (`backend/handlers.go`), reject any tenant credential that
  resolves to a dataset name starting with `_`. This makes `_system`
  unreachable via `/api/{contentType}` and `/api/datasets/{name}`.
- The **only** door into `_system` is a dedicated, `admin`-gated route group
  (see §8).

## 8. HTTP surface

All routes `admin`-only via the existing `protect(handler, "admin")` helper.

```
GET    /api/system/users
POST   /api/system/users
DELETE /api/system/users/{id}

GET    /api/system/keys
POST   /api/system/keys          # returns plaintext key once
DELETE /api/system/keys/{id}     # sets revoked_at, re-merges credentials
```

`openapi.yaml` is the source of truth for the HTTP contract and must be updated
alongside these handlers.

## 9. Implementation steps

1. **`backend/store/system.go`** — typed `_system` accessor: `SystemUsers()`,
   `SystemKeys()` CRUD over collections `"users"` / `"apikeys"`, reusing
   `Dataset.persistLocked` for atomic writes.
2. **Preload `_system` at startup** unconditionally in `main.go` (alongside the
   existing credential-dataset preload loop).
3. **Reserve `_` prefix** — reject tenant credentials binding to `_`-prefixed
   datasets in `auth` resolution and `datasetForRequest`.
4. **Hashing helper** — salted SHA-256 + constant-time compare; plaintext shown
   once; store `hash` + `prefix`.
5. **Merged credential layer in `auth`** — `BuildCredentials(env, systemRecords)`
   producing the merged maps with **env-over-`_system`** precedence; rebuilt on
   `_system` write; consumed by `NewVerifier` and `/api/login`.
6. **Bootstrap guard** — warn + env fallback when no admin exists.
7. **Routes/handlers** — `admin`-only `/api/system/users` and
   `/api/system/keys`.
8. **Docs** — update `openapi.yaml`; fix the stale persistence line in
   `CLAUDE.md`.
9. **Tests** — system CRUD, hash/verify, `_`-prefix rejection, **merge
   precedence (env over `_system`)**, and bootstrap fallback. (These would be the
   repo's first `_test.go` files.)

## 10. Open questions / future seams

- **JWT secret** could later move into `_system` (currently env-only).
- **Disk hot-reload**: we re-merge on in-process writes only; external edits to
  `data/_system/` need a restart.
- **Per-key scopes** finer than role+dataset (e.g. read-only keys).
