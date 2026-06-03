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

	// Auth session
	authed.HandleFunc("GET /api/auth/me", h.me)
	authed.HandleFunc("POST /api/auth/logout", h.logout)

	// Aggregate / dashboard
	authed.HandleFunc("GET /api/dashboard", h.dashboard)
	authed.HandleFunc("GET /api/summary", h.summary)
	authed.HandleFunc("GET /api/projects/{id}", h.projectByID)

	// Master data — registered uniformly via the generic CRUD factories.
	registerCRUD(authed, "funnel", handleList(h.svc.Funnel), handleCreate(h.svc.CreateFunnel), handleUpdate(h.svc.UpdateFunnel), handleDelete(h.svc.DeleteFunnel))
	registerCRUD(authed, "kpis", handleList(h.svc.KPIs), handleCreate(h.svc.CreateKPI), handleUpdate(h.svc.UpdateKPI), handleDelete(h.svc.DeleteKPI))
	registerCRUD(authed, "channels", handleList(h.svc.Channels), handleCreate(h.svc.CreateChannel), handleUpdate(h.svc.UpdateChannel), handleDelete(h.svc.DeleteChannel))
	registerCRUD(authed, "projects", handleList(h.svc.Projects), handleCreate(h.svc.CreateProject), handleUpdate(h.svc.UpdateProject), handleDelete(h.svc.DeleteProject))
	registerCRUD(authed, "assets", handleList(h.svc.Assets), handleCreate(h.svc.CreateAsset), handleUpdate(h.svc.UpdateAsset), handleDelete(h.svc.DeleteAsset))
	registerCRUD(authed, "ig-accounts", handleList(h.svc.IGAccounts), handleCreate(h.svc.CreateIGAccount), handleUpdate(h.svc.UpdateIGAccount), handleDelete(h.svc.DeleteIGAccount))
	registerCRUD(authed, "handover", handleList(h.svc.Handover), handleCreate(h.svc.CreateHandover), handleUpdate(h.svc.UpdateHandover), handleDelete(h.svc.DeleteHandover))
	registerCRUD(authed, "winning", handleList(h.svc.Winning), handleCreate(h.svc.CreateWinning), handleUpdate(h.svc.UpdateWinning), handleDelete(h.svc.DeleteWinning))
	registerCRUD(authed, "commands", handleList(h.svc.Commands), handleCreate(h.svc.CreateCommand), handleUpdate(h.svc.UpdateCommand), handleDelete(h.svc.DeleteCommand))
	registerCRUD(authed, "reason-codes", handleList(h.svc.ReasonCodes), handleCreate(h.svc.CreateReasonCode), handleUpdate(h.svc.UpdateReasonCode), handleDelete(h.svc.DeleteReasonCode))

	// Singletons (read + replace)
	authed.HandleFunc("GET /api/context", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, h.svc.Context()) })
	authed.HandleFunc("PUT /api/context", handleUpdateSingleton(h.svc.UpdateContext))
	authed.HandleFunc("GET /api/lead-quality", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, h.svc.LeadQuality()) })
	authed.HandleFunc("PUT /api/lead-quality", handleUpdateSingleton(h.svc.UpdateLeadQuality))
	authed.HandleFunc("GET /api/content", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, h.svc.Content()) })
	authed.HandleFunc("PUT /api/content", handleUpdateSingleton(h.svc.UpdateContent))
	authed.HandleFunc("GET /api/alerts", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, h.svc.Alerts()) })
	authed.HandleFunc("PUT /api/alerts", handleUpdateSingleton(h.svc.UpdateAlerts))

	// Mount the protected mux behind the auth middleware.
	mux.Handle("/api/", requireAuth(h.resolveUser)(authed))

	return chain(mux, logger, cors(allowOrigin))
}

// registerCRUD wires the four REST routes for a master-data resource.
func registerCRUD(mux *http.ServeMux, name string, list, create, update http.HandlerFunc, del http.HandlerFunc) {
	mux.HandleFunc("GET /api/"+name, list)
	mux.HandleFunc("POST /api/"+name, create)
	mux.HandleFunc("PUT /api/"+name+"/{id}", update)
	mux.HandleFunc("DELETE /api/"+name+"/{id}", del)
}
