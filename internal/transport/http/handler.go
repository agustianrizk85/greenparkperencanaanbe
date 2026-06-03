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

/* ---- Generic CRUD handler factories ----------------------------------- */

// handleList serves GET collection requests.
func handleList[T any](fn func() []T) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, fn())
	}
}

// handleCreate serves POST requests, decoding the body and returning 201.
func handleCreate[T any](fn func(T) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in T
		if !decodeJSON(w, r, &in) {
			return
		}
		out, err := fn(in)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// handleUpdate serves PUT /{id} requests.
func handleUpdate[T any](fn func(string, T) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in T
		if !decodeJSON(w, r, &in) {
			return
		}
		out, err := fn(r.PathValue("id"), in)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handleDelete serves DELETE /{id} requests, returning 204.
func handleDelete(fn func(string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(r.PathValue("id")); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleUpdateSingleton serves PUT requests for singleton resources (no id).
func handleUpdateSingleton[T any](fn func(T) (T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in T
		if !decodeJSON(w, r, &in) {
			return
		}
		out, err := fn(in)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

/* ---- Dashboard / aggregate -------------------------------------------- */

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "perencanaan"})
}

func (h *Handler) dashboard(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Dashboard())
}

func (h *Handler) summary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Summary())
}

func (h *Handler) projectByID(w http.ResponseWriter, r *http.Request) {
	project, err := h.svc.ProjectByID(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
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
