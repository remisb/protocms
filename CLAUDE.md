# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

ProtoCMS is a prototype headless CMS written in Go 1.26 using only the standard library plus a small `muxstack` middleware module. State is held in memory and persisted per dataset as a **v2 folder**: `data/<dataset>/{data.json,meta.json}` (`data.json` = schemas + content, `meta.json` = format version + timestamps). A legacy v1 single-file format (`data/<dataset>.json`) is still read for backward compatibility.

## Layout

The Go backend lives in `backend/` (its own module, `go.mod` there). A repo-root
`go.work` workspace points at `./backend` so the commands below run from the repo
root. The relative `data` directory is resolved from the working directory, so
**run from the repo root** to keep using the repo-root `data/` folder.

## Commands

All commands run from the repo root (the `go.work` workspace makes `./backend`
resolvable):

```sh
# Run with the default dataset (data/default/)
go run ./backend

# Run against a named dataset (creates/loads data/<name>/)
go run ./backend -dataset blog

# Build the binary
go build -o protocms ./backend

# Tests (none defined yet — the repo currently has no _test.go files)
go test ./backend/...
```

The server always listens on `:8080`. There is no config file or env-based port override.

## Architecture

Two packages: `main` (HTTP layer) and `store` (state + persistence). They are tightly coupled — handlers call `store` directly rather than going through an interface — so changes to the storage API ripple into `handlers.go`.

**Routing (`main.go`)** uses the Go 1.22+ `http.ServeMux` features: method-prefixed patterns (`GET /api/...`) and path wildcards (`{contentType}`, `{id}`). Handlers retrieve wildcards via `r.PathValue(...)`. The mux is wrapped by the external `muxstack/middleware` package (CORS → Logger → Recoverer); a rate limiter and timeout are commented out in `main.go` and can be re-enabled.

**Store (`store/`)** supports **multiple concurrent datasets** via a `Registry` (`store/registry.go`), each a `Dataset` (`store/dataset.go`) with its **own** `sync.RWMutex` — not one global lock. The package-level functions in `store/store.go` are a convenience facade over a single `defaultDataset`; the registry itself can hold many. Each `Dataset` holds:
- `schemas map[string]ContentType` — registered content type definitions
- `content map[string][]ContentItem` — content items keyed by content-type name (`ContentItem` is `map[string]any`, so items are schemaless at the Go level)
- `nextID int` — monotonic ID counter shared across all content types

`Init(dataset)` ensures the `data/` directory exists and must run before `Load(dataset)`. `openDataset` (`store/registry.go`) detects v2 folder vs. v1 flat-file format on load. Every mutation calls `persistLocked()` which writes a temp file then `os.Rename`s it over the data file — atomic, but synchronous on every write, so it is not suited to high throughput.

**Schema validation** happens only on `CreateContent`, not on `UpdateContent`. `FieldType` constants in `store.go` enumerate supported types (text, richText, number, boolean, date/datetime, image, media, select, reference, slug, json). New field types need both a constant and a case in `validateField`.

**Filtering (`listContentHandler` + `FilterContent`)** converts any non-reserved query param into an equality filter using `fmt.Sprintf("%v", val)`. The only reserved param is `limit`. Add reserved keys in `handlers.go` if you introduce sort/offset/etc.

**Auth (`internal/auth/auth.go`, wired in `auth.go` + `main.go`)** All reads are public; all writes require a bearer token. Tokens are verified by `NewVerifier` against either a static API key or an HS256 JWT, and the muxstack `Authenticator`/`Authorizer` middleware (already in the `muxstack/middleware` package) enforce role. A credential also carries the **dataset** it is bound to (optional `:dataset` segment, default `default`); `WithDataset` middleware puts it in the request context, and `datasetForRequest` (`handlers.go`) resolves it to a loaded dataset. Roles are the hard-coded set `{admin, editor}`: `admin` can do everything; `editor` can write content + uploads but not create content types. Enforcement is **per-route** via the `protect(handler, roles...)` helper in `main.go` (the global `middleware.Chain` can't be used because reads must stay open). Config currently comes from env vars: `PROTOCMS_API_KEYS` (`key:role[:dataset],...`), `PROTOCMS_JWT_SECRET` (HMAC secret; empty disables JWT and `/api/login`), and `PROTOCMS_USERS` (`user:pass:role[:dataset],...` for `/api/login`). JWT sign/verify is hand-rolled (stdlib `crypto/hmac`), no external library. Adding a new role means updating the valid-roles set in `internal/auth/auth.go` and the relevant `protect(...)` call sites.

A planned **`_system` dataset** will become the source of truth for users and API keys (env vars demoted to a debug override). See [`backend/docs/system-data-store.md`](backend/docs/system-data-store.md) for the design.

## Conventions

- Errors to clients are written as raw JSON strings via `http.Error` (e.g. `` `{"error":"..."}` ``), not through `jsonResponse`. Keep that pattern when adding handlers — clients parse it as JSON.
- Exported `store` functions hold the lock themselves. Do not call them while already holding `mu`; for internal call sites use the `*Locked` helpers (currently only `persistLocked`) or refactor.
- `openapi.yaml` is the source of truth for the HTTP contract — update it alongside handler changes.
- The compiled `protocms` binary is checked into the repo root. Rebuild it deliberately, not as a side effect.
