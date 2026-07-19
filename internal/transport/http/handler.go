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

// startTaskAI kicks off Deep Analisis AI on a task's review PDF (vision QC vs the
// selected checklist skill(s)). The vision check runs via auth's central key —
// forward the caller's token. Body is optional: { "skills": ["name", …] }.
func (h *Handler) startTaskAI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Skills []string `json:"skills"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	if err := h.svc.StartTaskAI(r.PathValue("id"), r.PathValue("taskId"), bearerToken(r), in.Skills); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "running"})
}

// taskAIStatus returns the task's Deep Analisis state (progress + findings).
func (h *Handler) taskAIStatus(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.TaskAIStatus(r.PathValue("id"), r.PathValue("taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// taskAIPDF serves the annotated Deep Analisis result PDF for a task.
func (h *Handler) taskAIPDF(w http.ResponseWriter, r *http.Request) {
	data, name, ok := h.svc.TaskAIAnnotated(r.PathValue("id"), r.PathValue("taskId"))
	if !ok {
		writeServiceError(w, service.ErrNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

/* ---- Skills (multi checklist markdown for the AI features) --------------- */

func (h *Handler) skillsList(w http.ResponseWriter, _ *http.Request) {
	list, err := h.svc.ListSkills()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) skillGet(w http.ResponseWriter, r *http.Request) {
	content, err := h.svc.ReadSkill(r.PathValue("name"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": r.PathValue("name"), "content": content})
}

// skillManager gates skill mutations to Kadep / CEO / Dirops.
func skillManager(r *http.Request) bool {
	user, _ := userFromContext(r.Context())
	return user.Role == domain.RoleKadep || user.Role == domain.RoleCEO || user.Role == domain.RoleDirops
}

func (h *Handler) skillPut(w http.ResponseWriter, r *http.Request) {
	if !skillManager(r) {
		writeServiceError(w, service.ErrForbidden)
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.WriteSkill(r.PathValue("name"), in.Content); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": r.PathValue("name"), "content": in.Content, "status": "ok"})
}

func (h *Handler) skillCreate(w http.ResponseWriter, r *http.Request) {
	if !skillManager(r) {
		writeServiceError(w, service.ErrForbidden)
		return
	}
	var in struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	meta, err := h.svc.CreateSkill(in.Name, in.Title)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, meta)
}

func (h *Handler) skillDelete(w http.ResponseWriter, r *http.Request) {
	if !skillManager(r) {
		writeServiceError(w, service.ErrForbidden)
		return
	}
	if err := h.svc.DeleteSkill(r.PathValue("name")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

/* ---- Deep Revisi AI (GK Kontraktor vs GK TTD vision check) ------------- */

func (h *Handler) uploadGKDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	kind := r.PathValue("kind")
	if err := r.ParseMultipartForm(22 << 20); err != nil {
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
	v, err := h.svc.UploadGKDoc(user, r.PathValue("id"), kind, header.Filename, data)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *Handler) getGKDoc(w http.ResponseWriter, r *http.Request) {
	data, name, err := h.svc.GKDocBytes(r.PathValue("id"), r.PathValue("kind"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) startDeepRevisi(w http.ResponseWriter, r *http.Request) {
	// The vision check runs via auth's central key — forward the caller's token.
	if err := h.svc.StartDeepRevisi(r.PathValue("id"), bearerToken(r)); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "running"})
}

func (h *Handler) deepRevisiStatus(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GKStatus(r.PathValue("id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// gkConfigGet reports the Deep Revisi status for the modal (read-only): whether
// the CENTRAL Kunci AI is set + the general and VISION models — all managed in
// Panel Admin → Kunci AI.
func (h *Handler) gkConfigGet(w http.ResponseWriter, r *http.Request) {
	keyConfigured, keyModel, visionModel := h.svc.GKKeyStatus(bearerToken(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"keyConfigured": keyConfigured,
		"keyModel":      keyModel,
		"visionModel":   visionModel,
	})
}

// gkSkillGet returns the editable Deep Revisi checklist markdown (the "skill"
// the vision AI follows). `fromFile` is false when the built-in default is shown.
func (h *Handler) gkSkillGet(w http.ResponseWriter, _ *http.Request) {
	content, fromFile := h.svc.GKSkillContent()
	writeJSON(w, http.StatusOK, map[string]any{"content": content, "fromFile": fromFile})
}

// gkSkillSet saves the checklist markdown. Managers only. Takes effect on the
// next Deep Revisi run (hot-editable).
func (h *Handler) gkSkillSet(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if user.Role != domain.RoleKadep && user.Role != domain.RoleCEO && user.Role != domain.RoleDirops {
		writeServiceError(w, service.ErrForbidden)
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.SaveGKSkill(in.Content); err != nil {
		writeServiceError(w, err)
		return
	}
	content, fromFile := h.svc.GKSkillContent()
	writeJSON(w, http.StatusOK, map[string]any{"content": content, "fromFile": fromFile, "status": "ok"})
}

/* ---- Staff / team ------------------------------------------------------ */

func (h *Handler) staff(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Staff(bearerToken(r)))
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

func (h *Handler) master(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Master(bearerToken(r)))
}

/* ---- GP + building-type masters (Fase 1) — CEO/Kadep manage ---- */

func (h *Handler) saveGP(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in domain.GP
	if !decodeJSON(w, r, &in) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		in.ID = id
	}
	gp, err := h.svc.SaveGP(user.Role, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gp)
}

func (h *Handler) deleteGP(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteGP(user.Role, r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) saveBuildingType(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in domain.BuildingType
	if !decodeJSON(w, r, &in) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		in.ID = id
	}
	t, err := h.svc.SaveBuildingType(user.Role, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) deleteBuildingType(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteBuildingType(user.Role, r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) saveLebar(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in domain.Lebar
	if !decodeJSON(w, r, &in) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		in.ID = id
	}
	l, err := h.svc.SaveLebar(user.Role, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}
func (h *Handler) deleteLebar(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteLebar(user.Role, r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) saveLokasi(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in domain.Lokasi
	if !decodeJSON(w, r, &in) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		in.ID = id
	}
	l, err := h.svc.SaveLokasi(user.Role, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}
func (h *Handler) deleteLokasi(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteLokasi(user.Role, r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ---- Blok + Kavling (Fase 2) ---- */

func (h *Handler) saveBlok(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in domain.Blok
	if !decodeJSON(w, r, &in) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		in.ID = id
	}
	// projectId comes from the path for creation; empty on update (blok keeps it).
	b, err := h.svc.SaveBlok(user.Role, r.PathValue("projectId"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *Handler) deleteBlok(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteBlok(user.Role, r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) saveKavling(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in domain.Kavling
	if !decodeJSON(w, r, &in) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		in.ID = id
	}
	k, err := h.svc.SaveKavling(user.Role, r.PathValue("projectId"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (h *Handler) deleteKavling(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteKavling(user.Role, r.PathValue("id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
