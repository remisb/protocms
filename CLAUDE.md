# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

ProtoCMS is a prototype headless CMS written in Go 1.26 using only the standard library plus a small `muxstack` middleware module. State is held in memory and persisted to a single JSON file per dataset under `data/<dataset>.json`.

## Commands

```sh
# Run with the default dataset (data/default.json)
go run .

# Run against a named dataset (creates/loads data/<name>.json)
go run . -dataset blog

# Build the binary
go build -o protocms .

# Tests (none defined yet — the repo currently has no _test.go files)
go test ./...
```

The server always listens on `:8080`. There is no config file or env-based port override.

## Architecture

Two packages: `main` (HTTP layer) and `store` (state + persistence). They are tightly coupled — handlers call `store` directly rather than going through an interface — so changes to the storage API ripple into `handlers.go`.

**Routing (`main.go`)** uses the Go 1.22+ `http.ServeMux` features: method-prefixed patterns (`GET /api/...`) and path wildcards (`{contentType}`, `{id}`). Handlers retrieve wildcards via `r.PathValue(...)`. The mux is wrapped by the external `muxstack/middleware` package (CORS → Logger → Recoverer); a rate limiter and timeout are commented out in `main.go` and can be re-enabled.

**Store (`store/store.go`)** is a process-global singleton guarded by one `sync.RWMutex`. Three maps make up the state:
- `schemas map[string]ContentType` — registered content type definitions
- `contentStore map[string][]ContentItem` — content items keyed by content-type name (`ContentItem` is `map[string]any`, so items are schemaless at the Go level)
- `nextID int` — monotonic ID counter shared across all content types

`Init(dataset)` resolves the data file path and must run before `Load(dataset)`. Every mutation calls `persistLocked()` which writes a temp file in `data/` then `os.Rename`s it over `dataFile` — atomic, but synchronous on every write, so it is not suited to high throughput.

**Schema validation** happens only on `CreateContent`, not on `UpdateContent`. `FieldType` constants in `store.go` enumerate supported types (text, richText, number, boolean, date/datetime, image, media, select, reference, slug, json). New field types need both a constant and a case in `validateField`.

**Filtering (`listContentHandler` + `FilterContent`)** converts any non-reserved query param into an equality filter using `fmt.Sprintf("%v", val)`. The only reserved param is `limit`. Add reserved keys in `handlers.go` if you introduce sort/offset/etc.

**Auth (`auth.go`)** All reads are public; all writes require a bearer token. Tokens are verified by `newVerifier` against either a static API key or an HS256 JWT, and the muxstack `Authenticator`/`Authorizer` middleware (already in the `muxstack/middleware` package) enforce role. Roles are the hard-coded set `{admin, editor}`: `admin` can do everything; `editor` can write content + uploads but not create content types. Enforcement is **per-route** via the `protect(handler, roles...)` helper in `main.go` (the global `middleware.Chain` can't be used because reads must stay open). Config comes from env vars, not the dataset: `PROTOCMS_API_KEYS` (`key:role,...`), `PROTOCMS_JWT_SECRET` (HMAC secret; empty disables JWT and `/api/login`), and `PROTOCMS_USERS` (`user:pass:role,...` for `/api/login`). JWT sign/verify is hand-rolled (stdlib `crypto/hmac`), no external library. Adding a new role means updating `validRoles` in `auth.go` and the relevant `protect(...)` call sites.

## Conventions

- Errors to clients are written as raw JSON strings via `http.Error` (e.g. `` `{"error":"..."}` ``), not through `jsonResponse`. Keep that pattern when adding handlers — clients parse it as JSON.
- Exported `store` functions hold the lock themselves. Do not call them while already holding `mu`; for internal call sites use the `*Locked` helpers (currently only `persistLocked`) or refactor.
- `openapi.yaml` is the source of truth for the HTTP contract — update it alongside handler changes.
- The compiled `protocms` binary is checked into the repo root. Rebuild it deliberately, not as a side effect.
