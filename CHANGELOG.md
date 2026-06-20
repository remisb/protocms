# Changelog

All notable changes to ProtoCMS are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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

#### Changed

- **All content access now requires authentication.** Reads
  (`GET /api/stats`, `/api/content-types`, `/api/content/...`) that were
  previously public now require a bearer token, like writes. Only
  `GET /api/health` and `POST /api/login` remain public. **(Breaking)**
- `GET /api/me` now returns the credential's bound `dataset`.
- Content routes are unchanged in shape; the dataset is determined by the
  authenticated API key or JWT rather than by URL or a default.
- `openapi.yaml` updated for the auth-on-reads change, the dataset routes, and
  the new metadata/metrics schemas.

#### Notes

- Existing `PROTOCMS_API_KEYS` / `PROTOCMS_USERS` entries without a dataset
  segment keep working (they bind to `default`).
- Uploads remain a single shared `data/uploads/` directory (not yet
  dataset-scoped).
- No data files are deleted by migration; cleanup of legacy `data/<name>.json`
  files is left to the operator.
