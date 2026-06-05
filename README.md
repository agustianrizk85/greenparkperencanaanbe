# Perencanaan API — Design Readiness Control Tower

Go (stdlib-only) backend for the Greenpark **Perencanaan** (planning) department
dashboard — the **Design Readiness Control Tower**. It tracks every unit through
the build-readiness pipeline (Akad → TTD Konsumen → SPK → Mulai Bangun), design
gates, drafting-team capacity and early-warning bottlenecks.

The data set is a snapshot exported from the department's Excel monitors
(*Progres Monitor Persiapan Pembangunan GP* + *Data Master Proyek*): 32 master
projects, 352 units, and a project code map. It is **embedded** in the binary
(`internal/repository/data.json`, via `go:embed`) and served **verbatim**; the
readiness model (pipeline, gates, capacity, alerts, commands) is derived on the
client (see `frontend/perencanaan/src/lib/compute.ts`).

## Architecture

```
cmd/server            composition root — wires repository -> service -> HTTP
internal/
  config              env-based configuration (PERENCANAAN_PORT, ...)
  auth                PBKDF2 password hashing + in-memory bearer-token sessions
  domain              User entity (auth)
  repository          embedded data.json snapshot + seeded user accounts
  service             auth use-cases + raw data accessor
  transport/http      router, handlers, middleware, JSON helpers
```

## Auth

- `POST /api/auth/login` → `{ token, user }`. Send `Authorization: Bearer <token>` on protected calls.
- Default accounts: **admin / admin123** and **viewer / viewer123** (change in any real deployment).

## API

| Method · Path            | Description                                            |
| ------------------------ | ----------------------------------------------------- |
| `GET /api/health`        | Liveness probe (public)                               |
| `POST /api/auth/login`   | Login (public)                                        |
| `GET /api/auth/me`       | Current user (auth)                                   |
| `POST /api/auth/logout`  | Revoke token (auth)                                   |
| `GET /api/data`          | Raw planning snapshot `{ today, projects, units, codeMap }` (auth) |

## Run

```bash
cd backend/perencanaan
go run ./cmd/server
# perencanaan API listening on http://localhost:8082
```

| Variable                    | Default | Description         |
| --------------------------- | ------- | ------------------- |
| `PERENCANAAN_PORT`          | `8082`  | HTTP port           |
| `PERENCANAAN_ALLOW_ORIGIN`  | `*`     | CORS allowed origin |

To refresh the data, replace `internal/repository/data.json` with a new export
and rebuild.
