# ProtoCMS

ProtoCMS is a prototype headless CMS written in Go 1.26 using only the standard
library plus a small `muxstack` middleware module, with a React admin frontend.
State is held in memory and persisted to disk per dataset. It is a work in
progress.

## Highlights

- **Multiple datasets** loaded in memory at once, each with its own metadata,
  query metrics, and uploads folder.
- **Per-dataset on-disk format:** `data/<name>/{data.json,meta.json,uploads/}`
  (legacy flat files `data/<name>.json` still load, with a migrate warning).
- **Authentication required for all access.** Reads and writes both need a
  bearer token (API key or JWT); the dataset a request targets is resolved from
  the credential. Only `GET /api/health` and `POST /api/login` are public.
- **Roles:** `admin` (everything) and `editor` (content + uploads, but not
  content-type creation or dataset management).
- React + Vite + Tailwind admin UI in `frontend/`.

See [`CHANGELOG.md`](./CHANGELOG.md) for the full feature history and
[`openapi.yaml`](./openapi.yaml) for the HTTP contract.

## Requirements

- Go 1.26+
- Node + [pnpm](https://pnpm.io/) (for the frontend)

## Running

### Backend

The Go backend lives in `backend/` (its own module). A repo-root `go.work`
workspace makes `./backend` resolvable, and the relative `data/` directory is
read from the working directory — so **run these commands from the repo root**.

```sh
# Default dataset (data/default/ or legacy data/default.json)
go run ./backend

# A named dataset
go run ./backend -dataset blog

# Build a binary instead
go build -o protocms ./backend && ./protocms -dataset blog
```

The server always listens on **`:8080`**. There is no config file or port
override.

#### Authentication / configuration (env vars)

All content access requires a credential, so set at least one of these before
hitting the API. The optional trailing `dataset` segment binds a credential to
a dataset (defaults to `default`).

| Variable | Format | Purpose |
| --- | --- | --- |
| `PROTOCMS_API_KEYS` | `key:role[:dataset],…` | Static API keys sent as `Authorization: Bearer <key>`. |
| `PROTOCMS_USERS` | `user:pass:role[:dataset],…` | Username/password logins for `POST /api/login`. |
| `PROTOCMS_JWT_SECRET` | any string | HMAC secret for HS256 JWTs. Empty disables `/api/login`. |

`role` is `admin` or `editor`. Datasets referenced by any credential are
**preloaded at startup**.

Example — an admin API key and a login user, both bound to the `menu` dataset:

```sh
PROTOCMS_JWT_SECRET=dev-secret \
PROTOCMS_API_KEYS="k-admin:admin:menu" \
PROTOCMS_USERS="boss:adminpw:admin:menu" \
go run ./backend -dataset menu
```

Then:

```sh
# Static API key
curl http://localhost:8080/api/stats -H "Authorization: Bearer k-admin"

# Or log in for a JWT and use that
TOKEN=$(curl -s -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"boss","password":"adminpw"}' | jq -r .token)
curl http://localhost:8080/api/content/dish -H "Authorization: Bearer $TOKEN"
curl http://localhost:8080/api/me -H "Authorization: Bearer $TOKEN"   # shows the bound dataset
```

Uploads referenced by the dataset are moved into `data/blog/uploads/` and their
stored URLs are rewritten. The command prints a summary and exits without
starting the server.

### Frontend

```sh
cd frontend
pnpm install
pnpm dev        # http://localhost:5173, proxies /api → http://localhost:8080
```

Run the backend (with auth configured) in another terminal. Log in through the
UI with a `PROTOCMS_USERS` account. Build the production bundle with
`pnpm build`.

## Testing

There are currently **no Go test files**; `go test ./backend/...` is wired up and
ready for new tests:

```sh
go test ./backend/...              # run all package tests
go test ./backend/... -run TestX   # a single test by name
go test ./backend/... -race        # with the race detector
go vet ./backend/...               # static checks
gofmt -l .                 # list unformatted files
```

Frontend type-checking and build:

```sh
cd frontend
pnpm typecheck   # tsc -b --noEmit
pnpm build       # tsc -b && vite build
```

### Smoke-testing the API

```sh
export TOKEN=…   # an API key or JWT (see Authentication above)

curl -s http://localhost:8080/api/health
curl -s http://localhost:8080/api/stats        -H "Authorization: Bearer $TOKEN" | jq
curl -s http://localhost:8080/api/content-types -H "Authorization: Bearer $TOKEN" | jq

# List + filter content (filters are equality on any field)
curl -s "http://localhost:8080/api/content/dish?category_id=1" -H "Authorization: Bearer $TOKEN" | jq

# Create a content type (admin)
curl -s -X POST http://localhost:8080/api/content-types \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"post","fields":{"title":{"type":"text","required":true},"body":{"type":"richText"}}}'

# Create / read content
curl -s -X POST http://localhost:8080/api/content/post \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"title":"Hello","body":"World"}'
curl -s http://localhost:8080/api/content/post/1 -H "Authorization: Bearer $TOKEN" | jq

# Dataset management + metrics (admin)
curl -s http://localhost:8080/api/datasets -H "Authorization: Bearer $TOKEN" | jq
curl -s http://localhost:8080/api/metrics  -H "Authorization: Bearer $TOKEN" | jq
```

## Debugging

- **Logs.** The server logs structured `slog` lines to stdout (routes, dataset
  loads, persistence warnings). It prints the full route table at startup.
- **401 / 403.** A `401` means a missing/invalid token (remember reads need one
  now); a `403` means the token's role lacks permission (e.g. an `editor`
  creating a content type or touching `/api/datasets`).
- **409 on uploads / metadata.** The dataset is still in the legacy flat-file
  format — run `-migrate` first.
- **Inspect state on disk.** A dataset lives in `data/<name>/data.json`
  (`{next_id, schemas, content}`) and `data/<name>/meta.json`. Persistence is
  atomic (temp file + rename) on every write.
- **Delve.**

  ```sh
  dlv debug ./backend -- -dataset blog   # step through startup + handlers
  dlv test ./backend/store               # debug a package's tests
  ```

- **Verbose Go build / vet output.**

  ```sh
  go build -v ./backend/...
  go vet ./backend/...
  ```

- **Frontend.** Use the browser devtools network tab against the Vite dev
  server; failed `/api` calls show the backend's JSON error body. A `401`
  surfaces a "session expired" toast and returns you to the login screen.

## Architecture

Two Go packages plus the auth module:

- `main` — HTTP layer (`main.go`, `handlers.go`, `dataset_handlers.go`,
  `uploads.go`, `auth.go`). Uses Go 1.22+ `http.ServeMux` with method-prefixed
  patterns and path wildcards.
- `store` — datasets, persistence, metadata, metrics, migration. A `Registry`
  holds many `*Dataset` instances, each guarded by its own lock.
- `internal/auth` — credential config, JWT sign/verify, and dataset resolution.

`openapi.yaml` is the source of truth for the HTTP contract — keep it in sync
with handler changes.
