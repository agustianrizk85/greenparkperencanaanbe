package http

import (
	"net/http"

	"greenpark/perencanaan/internal/domain"
)

// xdivProjects returns a minimal project list for a cross-division linker (e.g.
// Legal Permit choosing which perencanaan project a lahan maps to). Any authed
// SSO user (any division) may read it.
func (h *Handler) xdivProjects(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": h.svc.XDivProjects()})
}

// xdivDeliverables returns deliverables routed to a division, optionally scoped
// to one project — e.g. ?division=legalpermit&projectId=gp-001 yields that
// project's Siteplan tasks (Output=legalpermit) with hasDoc + taskId so the
// caller can download via the existing board task-doc endpoint.
func (h *Handler) xdivDeliverables(w http.ResponseWriter, r *http.Request) {
	div := domain.Division(r.URL.Query().Get("division"))
	pid := r.URL.Query().Get("projectId")
	writeJSON(w, http.StatusOK, map[string]any{"items": h.svc.XDivDeliverables(pid, div)})
}
