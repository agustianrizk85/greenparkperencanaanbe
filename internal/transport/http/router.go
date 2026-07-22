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
	// Accepts ANY-division SSO tokens (it only broadcasts {rev}).
	mux.HandleFunc("GET /api/ws", h.ws)
	// Board attachment download/stream: public route that validates its own
	// token (Authorization header OR ?token= query) so <img>/<video> tags can
	// load files; serves with Range support for video seeking. Any-division
	// SSO tokens are accepted (resolveUserAny inside the handler).
	mux.HandleFunc("GET /api/board/attachments/{attId}", h.boardServeAttachment)
	// Formal-task PDFs served inline for the board's task cards. PUBLIC routes:
	// they self-validate a header OR ?token= query token (same pattern as the
	// attachment route) so <embed>/<iframe> can load them. Being more specific
	// than the /api/board/ mount, they take precedence for these exact GET paths.
	mux.HandleFunc("GET /api/board/task/{projectId}/{taskId}/doc", h.boardTaskDoc)
	mux.HandleFunc("GET /api/board/task/{projectId}/{taskId}/deep-analisis/pdf", h.boardTaskAIPDF)
	// Task attachment download/stream (multi-file, any type) — PUBLIC, self-validated
	// token (header OR ?token=), Range-served so <img>/<video>/<embed> preview and
	// video seeking work. More specific than /api/board/, so it wins these GETs.
	mux.HandleFunc("GET /api/board/task/{projectId}/{taskId}/attachments/{attId}", h.boardTaskServeAttachment)

	// --- Protected ---
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/auth/me", h.me)
	authed.HandleFunc("POST /api/auth/logout", h.logout)

	// Portfolio overview.
	authed.HandleFunc("GET /api/summary", h.summary)

	// Projects + deliverable tree (flow menambah proyek).
	authed.HandleFunc("GET /api/projects", h.listProjects)
	authed.HandleFunc("POST /api/projects", h.addProject)
	authed.HandleFunc("POST /api/projects/import", h.importProjects)
	authed.HandleFunc("POST /api/projects/{id}/tasks/import", h.importTasks)
	authed.HandleFunc("PATCH /api/projects/{id}/tasks/{taskId}/schedule", h.setTaskSchedule)
	authed.HandleFunc("GET /api/projects/{id}", h.getProject)
	authed.HandleFunc("DELETE /api/projects/{id}", h.deleteProject)
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
	authed.HandleFunc("POST /api/master/import", h.importMaster)

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
	authed.HandleFunc("POST /api/projects/{projectId}/kavling/import", h.importKavling)
	authed.HandleFunc("PATCH /api/projects/{projectId}/kavling/{id}", h.saveKavling)
	authed.HandleFunc("DELETE /api/projects/{projectId}/kavling/{id}", h.deleteKavling)

	// Full cicle Kanban board mirror (read for all; push sync CEO/Kadep only).
	authed.HandleFunc("GET /api/cicle-board", h.cicleBoard)
	authed.HandleFunc("POST /api/cicle-board", h.setCicleBoard)

	// Admin (CEO & Kadep only): seed sample data, reset process only, or rebuild master.
	authed.HandleFunc("POST /api/admin/seed", h.seedDemo)
	authed.HandleFunc("POST /api/admin/reset-proses", h.resetProses)
	authed.HandleFunc("POST /api/admin/reset-master", h.resetMaster)
	authed.HandleFunc("POST /api/admin/empty-all", h.emptyAll)

	// Department Kanban board ("Departemen Perencanaan") — Trello-style lists,
	// cards, labels, checklists, attachments (bytes on disk) and comments.
	// CROSS-DIVISION: the board lives on its OWN submux mounted at /api/board/
	// with requireAuth(h.resolveUserAny), so any valid SSO user from ANY
	// division may use it; everything else under /api/ keeps the strict
	// perencanaan-only check. Patterns are FULL paths (no StripPrefix), so the
	// Go 1.22 ServeMux matches them exactly after the mount dispatch.
	// The attachment GET is on the PUBLIC mux above (self-validated token).
	board := http.NewServeMux()
	board.HandleFunc("GET /api/board", h.boardGet)
	board.HandleFunc("POST /api/board/lists", h.boardAddList)
	board.HandleFunc("PATCH /api/board/lists/{listId}", h.boardPatchList)
	board.HandleFunc("DELETE /api/board/lists/{listId}", h.boardDeleteList)
	board.HandleFunc("POST /api/board/cards", h.boardAddCard)
	board.HandleFunc("GET /api/board/cards/{cardId}", h.boardGetCard)
	board.HandleFunc("PATCH /api/board/cards/{cardId}", h.boardPatchCard)
	board.HandleFunc("DELETE /api/board/cards/{cardId}", h.boardDeleteCard)
	board.HandleFunc("POST /api/board/cards/{cardId}/members", h.boardAddMember)
	board.HandleFunc("DELETE /api/board/cards/{cardId}/members/{username}", h.boardRemoveMember)
	board.HandleFunc("POST /api/board/labels", h.boardAddLabel)
	board.HandleFunc("PATCH /api/board/labels/{labelId}", h.boardPatchLabel)
	board.HandleFunc("DELETE /api/board/labels/{labelId}", h.boardDeleteLabel)
	board.HandleFunc("POST /api/board/cards/{cardId}/labels", h.boardCardAddLabel)
	board.HandleFunc("DELETE /api/board/cards/{cardId}/labels/{labelId}", h.boardCardRemoveLabel)
	board.HandleFunc("POST /api/board/cards/{cardId}/checklists", h.boardAddChecklist)
	board.HandleFunc("PATCH /api/board/cards/{cardId}/checklists/{clId}", h.boardPatchChecklist)
	board.HandleFunc("DELETE /api/board/cards/{cardId}/checklists/{clId}", h.boardDeleteChecklist)
	board.HandleFunc("POST /api/board/cards/{cardId}/checklists/{clId}/items", h.boardAddChecklistItem)
	board.HandleFunc("PATCH /api/board/cards/{cardId}/checklists/{clId}/items/{itemId}", h.boardPatchChecklistItem)
	board.HandleFunc("DELETE /api/board/cards/{cardId}/checklists/{clId}/items/{itemId}", h.boardDeleteChecklistItem)
	board.HandleFunc("POST /api/board/cards/{cardId}/attachments", h.boardUploadAttachment)
	board.HandleFunc("DELETE /api/board/cards/{cardId}/attachments/{attId}", h.boardDeleteAttachment)
	board.HandleFunc("POST /api/board/cards/{cardId}/comments", h.boardAddComment)
	board.HandleFunc("DELETE /api/board/cards/{cardId}/comments/{commentId}", h.boardDeleteComment)
	// Cek AI on a card attachment (PDF or image) — async job + status poll.
	board.HandleFunc("POST /api/board/cards/{cardId}/ai-check", h.boardStartAICheck)
	board.HandleFunc("GET /api/board/cards/{cardId}/ai-check", h.boardAICheckStatus)
	// Formal-task proxy: the board's task cards act on the caller's real tasks via
	// the SAME service methods as /api/projects/{id}/tasks/{taskId}/*. Mutations
	// flow through bumpOnWrite so every board refreshes in realtime. (The two PDF
	// GETs — doc + deep-analisis/pdf — live on the PUBLIC mux above for ?token=.)
	board.HandleFunc("PATCH /api/board/task/{projectId}/{taskId}", h.boardTaskUpdate)
	board.HandleFunc("POST /api/board/task/{projectId}/{taskId}/doc", h.boardTaskUploadDoc)
	board.HandleFunc("POST /api/board/task/{projectId}/{taskId}/approve", h.boardTaskApprove)
	board.HandleFunc("POST /api/board/task/{projectId}/{taskId}/reject", h.boardTaskReject)
	board.HandleFunc("POST /api/board/task/{projectId}/{taskId}/deep-analisis", h.boardTaskStartAI)
	board.HandleFunc("GET /api/board/task/{projectId}/{taskId}/deep-analisis", h.boardTaskAIStatus)
	// Task attachments (multi-file, any type, ≤1 GiB each). Upload + delete live on
	// the board submux (contributor); the GET is on the PUBLIC mux above (?token=).
	board.HandleFunc("POST /api/board/task/{projectId}/{taskId}/attachments", h.boardTaskUploadAttachment)
	board.HandleFunc("DELETE /api/board/task/{projectId}/{taskId}/attachments/{attId}", h.boardTaskDeleteAttachment)
	board.HandleFunc("POST /api/board/task/{projectId}/{taskId}/comments", h.boardTaskAddComment)
	board.HandleFunc("DELETE /api/board/task/{projectId}/{taskId}/comments/{commentId}", h.boardTaskDeleteComment)
	// Deep Analisis skills picker: the available skill names for the source picker.
	board.HandleFunc("GET /api/board/skills", h.boardSkills)

	// Cross-division read (e.g. Legal Permit pulling the perencanaan Siteplan):
	// minimal project list + deliverables routed to a division. Uses the same
	// ANY-division resolveUserAny auth as the board; mounted at /api/xdiv below.
	// The deliverable DOCUMENT downloads via the existing board task-doc route.
	board.HandleFunc("GET /api/xdiv/projects", h.xdivProjects)
	board.HandleFunc("GET /api/xdiv/units", h.xdivUnits)
	board.HandleFunc("GET /api/xdiv/deliverables", h.xdivDeliverables)

	boardChain := requireAuth(h.resolveUserAny)(bumpOnWrite(h.hub)(board))
	// Exact "/api/board" registration avoids the ServeMux implicit trailing-slash
	// redirect (a 301 would drop the Authorization header on some clients).
	mux.Handle("/api/board", boardChain)
	mux.Handle("/api/board/", boardChain)
	mux.Handle("/api/xdiv/", boardChain)

	// Mount the protected mux behind auth, then bump the realtime revision on
	// every successful write so all connected dashboards refresh instantly.
	mux.Handle("/api/", requireAuth(h.resolveUser)(bumpOnWrite(h.hub)(authed)))

	return chain(mux, logger, cors(allowOrigin))
}
