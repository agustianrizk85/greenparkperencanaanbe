package http

import (
	"net/http"

	"greenpark/perencanaan/internal/domain"
	"greenpark/perencanaan/internal/service"
)

// Handler holds the dependencies for the HTTP handlers.
type Handler struct {
	svc *service.Service
}

// NewHandler creates a Handler bound to the given service.
func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

/* ---- Health + data ----------------------------------------------------- */

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "perencanaan"})
}

// data serves the raw planning snapshot (today, projects, units, codeMap)
// byte-for-byte. The readiness model is computed on the client.
func (h *Handler) data(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.svc.Data())
}

/* ---- Auth -------------------------------------------------------------- */

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	token, user, err := h.svc.Login(in.Username, in.Password)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.svc.Logout(bearerToken(r))
	w.WriteHeader(http.StatusNoContent)
}

// resolveUser is passed to the auth middleware.
func (h *Handler) resolveUser(token string) (domain.User, bool) {
	return h.svc.UserByToken(token)
}
