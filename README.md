# Perencanaan API — Departemen Perencanaan

Go (stdlib-only) backend for the Greenpark **Perencanaan** (planning) department.
It models the department's real business process:

- **Flow menambah proyek** — every project is expanded into the full deliverable
  tree (Site Plan, Desain Unit Hunian, Desain Kawasan), each leaf owned by one of
  the three design authors and routed to a downstream division.
- **Flow membagi tugas** — tasks are divided by **PIC account** (author).
- **Output per divisi** — finished deliverables are routed to Legal, Marketing,
  Teknik, Konsumen and the CEO overview.
- **Flow gambar kerja** — the per-consumer working-drawing flow with two SLA
  gates: **gambar kerja konsumen 15 hari kerja** sejak info masuk, and **gambar
  kerja kontraktor 5 hari kerja** sejak TTD konsumen, plus a live alert board.
- **Revisi AI** — an AI-assisted revision analysis for a consumer working drawing
  (placeholder pending the production model integration).

The 32-project portfolio is seeded from `internal/repository/projects.json`
(embedded via `go:embed`). The store is **in-memory and mutable** — task status
changes, new projects and working-drawing flows live for the process lifetime.

## Authors & roles

| Account  | Role     | Responsibility                                   |
| -------- | -------- | ------------------------------------------------ |
| `ceo`    | ceo      | Full overview, may manage anything               |
| `kadep`  | kadep    | Manage projects & assignments                    |
| `randi`  | arsitek  | Desain + render (denah, tampak, maingate, dll.)  |
| `ananto` | arsitek  | Interior, fasos fasum, animasi                   |
| `agus`   | drafter  | Gambar kerja (IMB, infrastruktur, kontraktor)    |

## Architecture

```
cmd/server            composition root — wires repository -> service -> HTTP
internal/
  config              env-based configuration (PERENCANAAN_PORT, ...)
  auth                PBKDF2 password hashing + in-memory bearer-token sessions
  domain              User, Project, Task tree template, WorkDrawing, workday math
  repository          embedded projects.json + mutable in-memory store
  service             rollups, PIC assignment, division outputs, SLA flow, alerts
  transport/http      router, handlers, middleware, JSON helpers
```

## Auth

- `POST /api/auth/login` → `{ token, user }`. Send `Authorization: Bearer <token>` on protected calls.
- Default accounts: **ceo/ceo123 · kadep/kadep123 · randi/randi123 · ananto/ananto123 · agus/agus123** (change in any real deployment).
- Writes (add project, update task) require CEO/Kadep, or the **owning PIC** for their own task.

## API

| Method · Path                               | Description                                          |
| ------------------------------------------- | ---------------------------------------------------- |
| `GET /api/health`                           | Liveness probe (public)                              |
| `POST /api/auth/login`                      | Login (public)                                       |
| `GET /api/auth/me`                          | Current user (auth)                                  |
| `POST /api/auth/logout`                     | Revoke token (auth)                                  |
| `GET /api/summary`                          | Portfolio metrics, PIC workload, division readiness  |
| `GET /api/projects`                         | Project rollups (progress, status)                   |
| `POST /api/projects`                        | Add a project (CEO/Kadep) — instantiates the tree    |
| `GET /api/projects/{id}`                    | Project detail with full deliverable tree            |
| `PATCH /api/projects/{id}/tasks/{taskId}`   | Update a task's status                                |
| `GET /api/my-tasks[?pic=]`                  | Tasks for the caller (or a PIC, for managers)        |
| `GET /api/outputs`                          | Deliverables grouped by division                     |
| `GET /api/workdrawings`                     | Consumer working-drawing flows with SLA countdowns   |
| `POST /api/workdrawings`                    | Start a flow (15-hk SLA from info masuk)             |
| `PATCH /api/workdrawings/{id}`              | Advance a flow (`konsumen-selesai`/`ttd-konsumen`/`kontraktor-selesai`) |
| `POST /api/workdrawings/{id}/revisi`        | AI revision analysis (placeholder)                   |
| `GET /api/alerts`                           | Active SLA alerts, most-urgent first                 |
| `GET /api/staff`                            | Department roster + per-author workload              |
| `POST /api/admin/seed`                      | Fill sample data (CEO/Kadep) — "Isi Contoh"          |
| `POST /api/admin/reset`                     | Delete all dynamic data (CEO/Kadep) — "Hapus Semua"  |

## Sample data

On startup the server seeds a realistic sample set (varied task progress + a
spread of working-drawing flows across every SLA state) so the dashboard is
populated immediately. Set `PERENCANAAN_SEED_DEMO=false` to start empty. CEO /
Kadep can re-fill (`/api/admin/seed`) or wipe (`/api/admin/reset`) at any time
from the dashboard header.

## Run

```bash
cd backend/perencanaan
go run ./cmd/server
# perencanaan API listening on http://localhost:8082
```

| Variable                    | Default | Description                            |
| --------------------------- | ------- | -------------------------------------- |
| `PERENCANAAN_PORT`          | `8082`  | HTTP port                              |
| `PERENCANAAN_ALLOW_ORIGIN`  | `*`     | CORS allowed origin                    |
| `PERENCANAAN_SEED_DEMO`     | `true`  | Seed sample data on startup (`false` = empty) |

The deliverable tree is defined in `internal/domain/template.go`; edit it there to
change the business process. To change the seed portfolio, edit
`internal/repository/projects.json` and rebuild.
