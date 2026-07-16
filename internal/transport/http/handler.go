package http

import (
	"encoding/json"
	"io"
	"net/http"

	"greenpark/perencanaan/internal/authmw"
	"greenpark/perencanaan/internal/domain"
	"greenpark/perencanaan/internal/service"
)

// Handler holds the dependencies for the HTTP handlers.
type Handler struct {
	svc *service.Service
	hub *wsHub

	// sso accepts the unified dashboard's Ed25519 login token directly (one login,
	// no bridge). nil = SSO off. Set via SetSSO.
	sso *authmw.Verifier
}

// SetSSO wires the master-auth SSO verifier so requests may authenticate with
// the unified dashboard login token in addition to the native token.
func (h *Handler) SetSSO(v *authmw.Verifier) { h.sso = v }

// ssoUser verifies an SSO token and maps its claims to a perencanaan domain.User.
func (h *Handler) ssoUser(tok string) (domain.User, bool) {
	if h.sso == nil || tok == "" {
		return domain.User{}, false
	}
	c, err := h.sso.Verify(tok)
	if err != nil || !c.CanAccess("perencanaan") {
		return domain.User{}, false
	}
	role := c.Role("perencanaan")
	if role == "" || c.Super {
		role = domain.RoleKadep
	}
	return domain.User{Username: c.Username, Name: c.Name, Role: role}, true
}

// NewHandler creates a Handler bound to the given service.
func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc, hub: newWSHub()}
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
	if u, ok := h.svc.UserByToken(token); ok {
		return u, true
	}
	return h.ssoUser(token) // fall back to the unified SSO login token
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

func (h *Handler) addTask(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in service.AddTaskInput
	if !decodeJSON(w, r, &in) {
		return
	}
	detail, err := h.svc.AddTask(user.Role, r.PathValue("id"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (h *Handler) removeTask(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	detail, err := h.svc.RemoveTask(user.Role, r.PathValue("id"), r.PathValue("taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) reassignTask(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in service.ReassignTaskInput
	if !decodeJSON(w, r, &in) {
		return
	}
	detail, err := h.svc.ReassignTask(user.Role, r.PathValue("id"), r.PathValue("taskId"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

/* ---- Review flow: upload PDF, view, approve, reject -------------------- */

func (h *Handler) uploadTaskDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read file")
		return
	}
	detail, err := h.svc.UploadTaskDoc(user, r.PathValue("id"), r.PathValue("taskId"), header.Filename, data)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) getTaskDoc(w http.ResponseWriter, r *http.Request) {
	data, name, err := h.svc.TaskDoc(r.PathValue("id"), r.PathValue("taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) approveTask(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	detail, err := h.svc.ApproveTask(user, r.PathValue("id"), r.PathValue("taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) rejectTask(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	// Body is optional: a plain "Tolak" (and the planning module) send none; a
	// "Revisi" sends { "instruction": "…" }. Tolerate an empty/absent body.
	var in reviseRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	detail, err := h.svc.RejectTask(user, r.PathValue("id"), r.PathValue("taskId"), in.Instruction)
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

/* ---- Staff / team ------------------------------------------------------ */

func (h *Handler) staff(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Staff())
}

/* ---- Cicle board mirror (full Kanban synced from cicle) --------------- */

func (h *Handler) cicleBoard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(h.svc.CicleBoard())
}

func (h *Handler) setCicleBoard(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20)) // 32MB cap
	if err != nil {
		writeError(w, http.StatusBadRequest, "gagal membaca body")
		return
	}
	if err := h.svc.SetCicleBoard(user.Role, body); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(body)})
}

/* ---- Accounts (dynamic PIC management, CEO/Kadep) ---------------------- */

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	accounts, err := h.svc.Accounts(user.Role)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in service.CreateUserInput
	if !decodeJSON(w, r, &in) {
		return
	}
	acc, err := h.svc.CreateUser(user.Role, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, acc)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteUser(user.Role, r.PathValue("username")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": r.PathValue("username")})
}

/* ---- Master reference data --------------------------------------------- */

func (h *Handler) master(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Master())
}

/* ---- Admin: seed demo / reset process / reset master ------------------- */

func (h *Handler) seedDemo(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.SeedDemo(user.Role); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "seeded"})
}

func (h *Handler) resetProses(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.ResetProses(user.Role); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset-proses"})
}

func (h *Handler) resetMaster(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.ResetMaster(user.Role); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset-master"})
}
