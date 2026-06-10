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

/* ---- Health ------------------------------------------------------------ */

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "perencanaan"})
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

/* ---- Summary ----------------------------------------------------------- */

func (h *Handler) summary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Summary())
}

/* ---- Projects ---------------------------------------------------------- */

func (h *Handler) listProjects(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Projects())
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.Project(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) addProject(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in service.AddProjectInput
	if !decodeJSON(w, r, &in) {
		return
	}
	detail, err := h.svc.AddProject(user.Role, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

type updateTaskRequest struct {
	Status domain.TaskStatus `json:"status"`
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in updateTaskRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.UpdateTask(user, r.PathValue("id"), r.PathValue("taskId"), in.Status); err != nil {
		writeServiceError(w, err)
		return
	}
	detail, err := h.svc.Project(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

/* ---- Task assignment (by PIC) ------------------------------------------ */

// myTasks returns the authenticated user's assigned tasks, or — for a manager
// passing ?pic=<username> — that author's tasks.
func (h *Handler) myTasks(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	pic := r.URL.Query().Get("pic")
	if pic == "" {
		pic = user.Username
	}
	writeJSON(w, http.StatusOK, h.svc.TasksForPIC(pic))
}

/* ---- Outputs by division ----------------------------------------------- */

func (h *Handler) outputs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.OutputsByDivision())
}

/* ---- Work drawings + alerts -------------------------------------------- */

func (h *Handler) listWorkDrawings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.WorkDrawings())
}

func (h *Handler) createWorkDrawing(w http.ResponseWriter, r *http.Request) {
	var in service.CreateWorkDrawingInput
	if !decodeJSON(w, r, &in) {
		return
	}
	v, err := h.svc.CreateWorkDrawing(in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) advanceWorkDrawing(w http.ResponseWriter, r *http.Request) {
	var in service.AdvanceWorkDrawingInput
	if !decodeJSON(w, r, &in) {
		return
	}
	v, err := h.svc.AdvanceWorkDrawing(r.PathValue("id"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type reviseRequest struct {
	Instruction string `json:"instruction"`
}

func (h *Handler) reviseWorkDrawing(w http.ResponseWriter, r *http.Request) {
	var in reviseRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	v, err := h.svc.ReviseWorkDrawing(r.PathValue("id"), in.Instruction)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) alerts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Alerts())
}
