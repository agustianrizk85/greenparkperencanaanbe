# Perencanaan API — Qualified Demand Control Tower

Go (stdlib-only) backend for the Greenpark **Perencanaan** (planning / demand)
control tower. It serves the marketing-funnel command dashboard used by the CEO
war-room: funnel volume, North-Star KPIs, channel performance, lead quality,
per-project demand & readiness, the digital-asset registry, winning campaigns,
CEO commands and early-warning alerts.

The architecture mirrors the other Greenpark services (e.g. `backend/teknik`):
a clean **repository → service → HTTP transport** layering with an in-memory,
concurrency-safe store seeded with representative data, token-based auth
(PBKDF2 + opaque bearer tokens), and generic CRUD wiring for every master-data
resource. No external dependencies.

## Run

```bash
cd backend/perencanaan
go run ./cmd/server        # listens on http://localhost:8082
go test ./...              # service unit tests
```

Configuration (environment variables, with defaults):

| Variable                    | Default | Purpose                |
| --------------------------- | ------- | ---------------------- |
| `PERENCANAAN_PORT`          | `8082`  | HTTP listen port       |
| `PERENCANAAN_ALLOW_ORIGIN`  | `*`     | CORS allowed origin    |

## Auth

All `/api/*` routes except `GET /api/health` and `POST /api/auth/login` require
an `Authorization: Bearer <token>` header.

Seeded demo users:

| Username | Password   | Role               |
| -------- | ---------- | ------------------ |
| `admin`  | `admin123` | Kadep Perencanaan  |
| `mkt`    | `mkt12345` | Marketing          |

## API

### Session

| Method | Path               | Description                       |
| ------ | ------------------ | --------------------------------- |
| POST   | `/api/auth/login`  | `{username,password}` → token+user |
| GET    | `/api/auth/me`     | Current user                      |
| POST   | `/api/auth/logout` | Revoke the bearer token           |
| GET    | `/api/health`      | Liveness probe                    |

### Aggregate

| Method | Path                 | Description                                        |
| ------ | -------------------- | -------------------------------------------------- |
| GET    | `/api/dashboard`     | Full payload (all collections + derived `summary`) |
| GET    | `/api/summary`       | Derived executive summary only                     |
| GET    | `/api/projects/{id}` | Single project                                     |

### Master data (full CRUD: `GET` list, `POST`, `PUT /{id}`, `DELETE /{id}`)

`funnel`, `kpis`, `channels`, `projects`, `assets`, `ig-accounts`, `handover`,
`winning`, `commands`, `reason-codes`.

### Singletons (`GET` read + `PUT` replace)

`context`, `lead-quality`, `content`, `alerts`.

## Derived summary

`GET /api/summary` (also embedded in `/api/dashboard`) is computed server-side so
the headline numbers always reconcile with the editable master data:

- `totalLeads`, `totalMQL`, `totalSpend` — summed from the **channels** matrix
- `mqlRate` = `totalMQL / totalLeads`
- `cpl` = `totalSpend / totalLeads`, `costPerBooking` = `totalSpend / totalBooking`
- `totalBooking` — summed from **projects**
- `achievement` = `context.bookingYTD / context.goal`
- `redAlerts` — count of red **alerts**; `openCommands` — **commands** not `done`

## Layout

```
cmd/server/main.go              Composition root + graceful shutdown
internal/config                 Env-driven configuration
internal/auth                   PBKDF2 hashing + in-memory session store
internal/domain                 Core entities (the JSON contract)
internal/repository             Generic collection + in-memory store + seed
internal/service                Business logic, CRUD validation, summary, auth
internal/transport/http         Router, handlers, middleware, JSON responses
```
