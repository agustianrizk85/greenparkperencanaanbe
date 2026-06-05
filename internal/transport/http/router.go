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
	authed.HandleFunc("GET /api/data", h.data)

	// Mount the protected mux behind the auth middleware.
	mux.Handle("/api/", requireAuth(h.resolveUser)(authed))

	return chain(mux, logger, cors(allowOrigin))
}
