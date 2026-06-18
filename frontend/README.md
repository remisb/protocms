# ProtoCMS Frontend

Vite + React 19 + TypeScript + Tailwind v4 + shadcn-style UI.

## Setup

```sh
pnpm install
```

## Develop

Start the Go API first (from the repo root):

```sh
go run .
```

Then the frontend:

```sh
pnpm dev
```

The dev server runs on `http://localhost:5173` and proxies `/api/*` to
`http://localhost:8080`, so the browser only ever talks to Vite.

## Routes

- `/designer` — manage content type schemas
- `/editor` — list/create/update/delete content items for a chosen type
- `/editor/:contentType` — deep-link straight to a type's editor

## Build

```sh
pnpm build
```
