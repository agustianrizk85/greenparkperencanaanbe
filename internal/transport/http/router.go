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

	// Mount the protected mux behind the auth middleware.
	mux.Handle("/api/", requireAuth(h.resolveUser)(authed))

	return chain(mux, logger, cors(allowOrigin))
}
