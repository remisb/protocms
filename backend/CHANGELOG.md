# Changelog

All notable changes to ProtoCMS are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.0.1] - 2026-06-21

First tagged release. Contains the data-store redesign and the `_system`
credential store described below.

### System data store (`_system`)

Added a reserved `_system` dataset that is the source of truth for user
accounts and API keys, with the environment-variable credentials demoted to a
debug/testing override. Internal-only and always loaded at startup.

#### Added

- **`_system` dataset.** A reserved dataset (stored like any other at
  `data/_system/{data.json,meta.json}`) holding two collections — `users` and
  `apikeys`. It is always loaded at startup.
- **Credentials in `_system`, env as override.** `_system` users and keys are
  the primary credential store; `PROTOCMS_API_KEYS` / `PROTOCMS_USERS` still
  work and **win on conflict** (env-over-`_system`), so they remain a usable
  debug/testing escape hatch. The merged credential set is rebuilt on every
  `_system` write, so changes take effect in-process without a restart.
- **Hashed at rest.** API keys and user passwords are stored as salted SHA-256
  hashes; a new key's plaintext is returned exactly once at creation and never
  persisted. Hashes are never exposed through the API.
- **Admin-only management API** (admin only):
  - `GET` / `POST /api/system/users`, `DELETE /api/system/users/{id}`.
  - `GET` / `POST /api/system/keys` (POST returns the one-time plaintext key),
    `DELETE /api/system/keys/{id}` (revokes; the record is retained).
- **Internal-only isolation.** Dataset names with a `_` prefix are reserved:
  the tenant content API rejects any credential bound to one, and the
  credential-management handlers refuse to create a credential bound to a
  reserved dataset. `/api/system/*` is the only door into `_system`.
- **Bootstrap guard.** At startup, if no admin credential exists in either an
  env API key or `_system`, a warning is logged (env always overrides
  `_system`, so an env admin key is a guaranteed way in).
- **`openapi.yaml`** documents the `/api/system/*` routes and their schemas.

#### Notes

- A `_system` user authenticates via `POST /api/login` like any user; the
  issued JWT carries the user's bound dataset in its `ds` claim.
- A malformed/hand-edited `_system` record that fails to decode is skipped with
  a logged warning rather than dropped silently.

### Data store redesign

Reworked the data store from a single process-global dataset into a registry
of independently-loadable datasets, each with metadata, query metrics, and a
new on-disk layout. The dataset a request targets is now resolved from its
credential.

#### Added

- **Multiple datasets in memory at once.** A `Registry` holds many `Dataset`
  instances, each guarded by its own lock, so datasets are served
  concurrently without a global lock.
- **Per-dataset folder format (v2).** Each dataset is stored as
  `data/<name>/{data.json,meta.json}`. Legacy flat files (`data/<name>.json`)
  still load and serve in place, with a warning to migrate.
- **Dataset metadata** (`meta.json`): name, author, description, created/last
  modified timestamps, data-format version, schema version, and tags.
- **`-migrate` flag.** `protocms -migrate -dataset <name>` converts a dataset
  from the legacy flat file to the v2 folder format and exits. It never
  deletes the old file and refuses to overwrite an existing folder.
- **Per-dataset query metrics** — counts and latency (avg/total) bucketed by
  operation (list/get/create/update/delete/filter) and content type — plus an
  estimated in-memory size.
- **Credential-bound datasets.** `PROTOCMS_API_KEYS` accepts
  `key:role[:dataset]` and `PROTOCMS_USERS` accepts `user:pass:role[:dataset]`
  (dataset optional, defaults to `default`); JWTs carry a `ds` claim. Each
  request's dataset is resolved from its token.
- **Admin dataset-management API** (admin only):
  - `GET /api/datasets` — list loaded datasets with metadata, memory, and a
    metrics summary.
  - `GET /api/datasets/{name}` — one dataset's metadata + full metrics.
  - `POST /api/datasets/{name}/load` / `…/unload` — load or unload a dataset
    at runtime (unload leaves files on disk).
  - `PATCH /api/datasets/{name}` — edit author/description/tags/schema version.
  - `GET /api/metrics` — the caller's dataset info (stats + memory + metrics).
- **Frontend datasets view** (admin only): a `/datasets` page listing loaded
  datasets with size and query metrics, runtime load/unload, and an editable
  metadata form. The sidebar account header now shows the credential's
  dataset, and a "session expired" toast surfaces when a token is rejected
  mid-session.
- **Per-dataset uploads.** Uploaded files now live in
  `data/<dataset>/uploads/`. Uploading (`POST /api/uploads`) targets the
  dataset bound to the credential and is only allowed for migrated (v2)
  datasets. `-migrate` moves any shared uploads referenced by a dataset into
  its folder and rewrites the stored URLs.
- **Credential datasets are preloaded at startup.** Every dataset referenced
  by a configured API key or user is loaded into memory when the server
  starts, so the first request for each doesn't pay the load cost.

#### Changed

- **All content access now requires authentication.** Reads
  (`GET /api/stats`, `/api/content-types`, `/api/content/...`) that were
  previously public now require a bearer token, like writes. Only
  `GET /api/health` and `POST /api/login` remain public. **(Breaking)**
- `GET /api/me` now returns the credential's bound `dataset`.
- Content routes are unchanged in shape; the dataset is determined by the
  authenticated API key or JWT rather than by URL or a default.
- Upload serving moved from `GET /api/uploads/{name}` to
  `GET /api/uploads/{dataset}/{name}` (still public, so plain `<img>` tags
  load images); the upload response `url` now includes the dataset segment.
  **(Breaking for stored upload URLs without a dataset segment — migration
  rewrites them.)**
- `openapi.yaml` updated for the auth-on-reads change, the dataset routes, and
  the new metadata/metrics schemas.

#### Notes

- Existing `PROTOCMS_API_KEYS` / `PROTOCMS_USERS` entries without a dataset
  segment keep working (they bind to `default`).
- No data files are deleted by migration; cleanup of legacy
  `data/<name>.json` files and the shared `data/uploads/` directory is left to
  the operator.
