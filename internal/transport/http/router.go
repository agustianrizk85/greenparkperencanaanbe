package http

import "net/http"

// NewRouter wires routes and applies global + per-scope middleware.
//
// Public routes:   GET /api/health, POST /api/auth/login.
// Everything else requires a valid bearer token (requireAuth).
func NewRouter(h *Handler, allowOrigin string) http.Handler {
	mux := http.NewServeMux()

	// --- Public ---
	mux.HandleFunc("GET /api/health", h.health)
	mux.HandleFunc("POST /api/auth/login", h.login)
	// Realtime push: validates its own ?token= (browsers can't set WS headers).
	mux.HandleFunc("GET /api/ws", h.ws)

	// --- Protected ---
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/auth/me", h.me)
	authed.HandleFunc("POST /api/auth/logout", h.logout)

	// Portfolio overview.
	authed.HandleFunc("GET /api/summary", h.summary)

	// Projects + deliverable tree (flow menambah proyek).
	authed.HandleFunc("GET /api/projects", h.listProjects)
	authed.HandleFunc("POST /api/projects", h.addProject)
	authed.HandleFunc("GET /api/projects/{id}", h.getProject)
	authed.HandleFunc("PATCH /api/projects/{id}/tasks/{taskId}", h.updateTask)
	// Dynamic deliverable structure editing (CEO/Kadep): add, remove, reassign.
	authed.HandleFunc("POST /api/projects/{id}/tasks", h.addTask)
	authed.HandleFunc("DELETE /api/projects/{id}/tasks/{taskId}", h.removeTask)
	authed.HandleFunc("PATCH /api/projects/{id}/tasks/{taskId}/assign", h.reassignTask)

	// Review flow: PIC uploads a PDF; Kadep approves (-> Selesai) or rejects.
	authed.HandleFunc("POST /api/projects/{id}/tasks/{taskId}/doc", h.uploadTaskDoc)
	authed.HandleFunc("GET /api/projects/{id}/tasks/{taskId}/doc", h.getTaskDoc)
	authed.HandleFunc("POST /api/projects/{id}/tasks/{taskId}/approve", h.approveTask)
	authed.HandleFunc("POST /api/projects/{id}/tasks/{taskId}/reject", h.rejectTask)
	// Deep Analisis AI on a task's review PDF (single-document vision QC).
	authed.HandleFunc("POST /api/projects/{id}/tasks/{taskId}/deep-analisis", h.startTaskAI)
	authed.HandleFunc("GET /api/projects/{id}/tasks/{taskId}/deep-analisis", h.taskAIStatus)
	authed.HandleFunc("GET /api/projects/{id}/tasks/{taskId}/deep-analisis/pdf", h.taskAIPDF)

	// Task assignment by PIC (flow membagi tugas).
	authed.HandleFunc("GET /api/my-tasks", h.myTasks)

	// Outputs routed to divisions.
	authed.HandleFunc("GET /api/outputs", h.outputs)

	// Per-consumer working-drawing flow + SLA alerts (flow gambar kerja).
	authed.HandleFunc("GET /api/workdrawings", h.listWorkDrawings)
	authed.HandleFunc("POST /api/workdrawings", h.createWorkDrawing)
	authed.HandleFunc("PATCH /api/workdrawings/{id}", h.advanceWorkDrawing)
	authed.HandleFunc("POST /api/workdrawings/{id}/revisi", h.reviseWorkDrawing)
	authed.HandleFunc("GET /api/alerts", h.alerts)

	// Deep Revisi AI: GK Kontraktor vs GK TTD vision check (Ollama Cloud).
	authed.HandleFunc("POST /api/workdrawings/{id}/gk/{kind}", h.uploadGKDoc)
	authed.HandleFunc("GET /api/workdrawings/{id}/gk/{kind}", h.getGKDoc)
	authed.HandleFunc("POST /api/workdrawings/{id}/deep-revisi", h.startDeepRevisi)
	authed.HandleFunc("GET /api/workdrawings/{id}/deep-revisi", h.deepRevisiStatus)
	// Deep Revisi AI status (central Kunci AI + vision model, read-only).
	authed.HandleFunc("GET /api/gk/config", h.gkConfigGet)
	// Deep Revisi AI "skill" (the editable checklist markdown the vision AI follows).
	authed.HandleFunc("GET /api/gk/skill", h.gkSkillGet)
	authed.HandleFunc("PUT /api/gk/skill", h.gkSkillSet)
	// Multi-skill: several editable checklists the AI features can pick from.
	authed.HandleFunc("GET /api/gk/skills", h.skillsList)
	authed.HandleFunc("POST /api/gk/skills", h.skillCreate)
	authed.HandleFunc("GET /api/gk/skills/{name}", h.skillGet)
	authed.HandleFunc("PUT /api/gk/skills/{name}", h.skillPut)
	authed.HandleFunc("DELETE /api/gk/skills/{name}", h.skillDelete)

	// Department roster / staff workload.
	authed.HandleFunc("GET /api/staff", h.staff)

	// Dynamic PIC account management (CEO / Kadep): list, create, delete.
	authed.HandleFunc("GET /api/users", h.listUsers)
	authed.HandleFunc("POST /api/users", h.createUser)
	authed.HandleFunc("DELETE /api/users/{username}", h.deleteUser)

	// Master reference data (projects, deliverable template, accounts, divisions).
	authed.HandleFunc("GET /api/master", h.master)

	// GP (grup) + building-type masters (Fase 1 of the relational project model).
	authed.HandleFunc("POST /api/gps", h.saveGP)
	authed.HandleFunc("PATCH /api/gps/{id}", h.saveGP)
	authed.HandleFunc("DELETE /api/gps/{id}", h.deleteGP)
	authed.HandleFunc("POST /api/building-types", h.saveBuildingType)
	authed.HandleFunc("PATCH /api/building-types/{id}", h.saveBuildingType)
	authed.HandleFunc("DELETE /api/building-types/{id}", h.deleteBuildingType)
	authed.HandleFunc("POST /api/lebars", h.saveLebar)
	authed.HandleFunc("PATCH /api/lebars/{id}", h.saveLebar)
	authed.HandleFunc("DELETE /api/lebars/{id}", h.deleteLebar)
	authed.HandleFunc("POST /api/lokasis", h.saveLokasi)
	authed.HandleFunc("PATCH /api/lokasis/{id}", h.saveLokasi)
	authed.HandleFunc("DELETE /api/lokasis/{id}", h.deleteLokasi)

	// Blok + Kavling per project (Fase 2). Project-scoped so projectId is always
	// known (needed to validate blok/type references on update).
	authed.HandleFunc("POST /api/projects/{projectId}/bloks", h.saveBlok)
	authed.HandleFunc("PATCH /api/projects/{projectId}/bloks/{id}", h.saveBlok)
	authed.HandleFunc("DELETE /api/projects/{projectId}/bloks/{id}", h.deleteBlok)
	authed.HandleFunc("POST /api/projects/{projectId}/kavling", h.saveKavling)
	authed.HandleFunc("PATCH /api/projects/{projectId}/kavling/{id}", h.saveKavling)
	authed.HandleFunc("DELETE /api/projects/{projectId}/kavling/{id}", h.deleteKavling)

	// Full cicle Kanban board mirror (read for all; push sync CEO/Kadep only).
	authed.HandleFunc("GET /api/cicle-board", h.cicleBoard)
	authed.HandleFunc("POST /api/cicle-board", h.setCicleBoard)

	// Admin (CEO & Kadep only): seed sample data, reset process only, or rebuild master.
	authed.HandleFunc("POST /api/admin/seed", h.seedDemo)
	authed.HandleFunc("POST /api/admin/reset-proses", h.resetProses)
	authed.HandleFunc("POST /api/admin/reset-master", h.resetMaster)

	// Mount the protected mux behind auth, then bump the realtime revision on
	// every successful write so all connected dashboards refresh instantly.
	mux.Handle("/api/", requireAuth(h.resolveUser)(bumpOnWrite(h.hub)(authed)))

	return chain(mux, logger, cors(allowOrigin))
}
