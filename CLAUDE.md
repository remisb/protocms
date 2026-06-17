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

## Conventions

- Errors to clients are written as raw JSON strings via `http.Error` (e.g. `` `{"error":"..."}` ``), not through `jsonResponse`. Keep that pattern when adding handlers — clients parse it as JSON.
- Exported `store` functions hold the lock themselves. Do not call them while already holding `mu`; for internal call sites use the `*Locked` helpers (currently only `persistLocked`) or refactor.
- `openapi.yaml` is the source of truth for the HTTP contract — update it alongside handler changes.
- The compiled `protocms` binary is checked into the repo root. Rebuild it deliberately, not as a side effect.
