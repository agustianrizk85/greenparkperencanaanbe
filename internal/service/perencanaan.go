// Package service holds the business logic of the Perencanaan (planning) module:
// the project deliverable tree, task assignment to the three design authors,
// the routing of finished deliverables to downstream divisions, and the
// per-consumer working-drawing (gambar kerja) flow with its SLA alerts.
package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/domain"
	"greenpark/perencanaan/internal/repository"
)

// Sentinel errors mapped to HTTP status codes by the transport layer.
var (
	ErrNotFound           = errors.New("resource not found")
	ErrValidation         = errors.New("validation failed")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrForbidden          = errors.New("not permitted")
)

// Service exposes the planning use-cases plus authentication.
type Service struct {
	repo      repository.Store
	sessions  *auth.SessionStore
	now       func() time.Time // injectable clock (defaults to time.Now)
	gk        GKConfig         // Deep Revisi AI — vision proxied through auth (central key + vision model)
	uploadDir string           // board attachment files live here (one file per attachment ID)

	boardRoster boardRosterCache // cross-division member roster (TTL cache, see boardroster.go)

	boardAIMu   sync.Mutex                    // guards boardAIJobs
	boardAIJobs map[string]*BoardAICheckState // card ID -> Cek AI job state (in-memory only)

	notify func() // optional realtime hook (WS rev bump) for background mutations
}

// New builds a Service from the store and session manager. repo may be the
// in-memory store or the Postgres-backed one — both satisfy repository.Store.
// gk configures the optional Deep Revisi AI feature (zero value = disabled).
// uploadDir is where board attachment files are stored on disk.
func New(repo repository.Store, sessions *auth.SessionStore, gk GKConfig, uploadDir string) *Service {
	return &Service{
		repo: repo, sessions: sessions, now: time.Now, gk: gk, uploadDir: uploadDir,
		boardAIJobs: map[string]*BoardAICheckState{},
	}
}

// SetChangeNotifier registers a callback fired after a BACKGROUND data change
// (e.g. the Cek AI auto-comment) so the transport can bump its realtime
// revision — bumpOnWrite only covers changes made inside an HTTP request.
func (s *Service) SetChangeNotifier(fn func()) { s.notify = fn }

// notifyChange fires the registered change notifier, if any.
func (s *Service) notifyChange() {
	if s.notify != nil {
		s.notify()
	}
}

// today returns the current date as YYYY-MM-DD.
func (s *Service) today() string { return s.now().Format(domain.DateLayout) }

/* ---- Auth -------------------------------------------------------------- */

// Login verifies credentials and issues a bearer token.
func (s *Service) Login(username, password string) (string, domain.User, error) {
	u, ok := s.repo.UserByUsername(username)
	if !ok || !auth.Verify(password, u.PasswordHash, u.Salt) {
		return "", domain.User{}, ErrInvalidCredentials
	}
	token, err := s.sessions.Issue(u.Username)
	if err != nil {
		return "", domain.User{}, err
	}
	return token, u, nil
}

// UserByToken resolves the user behind a valid session token.
func (s *Service) UserByToken(token string) (domain.User, bool) {
	username, ok := s.sessions.Resolve(token)
	if !ok {
		return domain.User{}, false
	}
	return s.repo.UserByUsername(username)
}

// Logout revokes a session token.
func (s *Service) Logout(token string) { s.sessions.Revoke(token) }

// canManage reports whether a role may add projects or edit any task.
func canManage(role string) bool {
	return role == domain.RoleCEO || role == domain.RoleDirops || role == domain.RoleKadep
}

/* ---- Projects ---------------------------------------------------------- */

// Projects returns the portfolio as rollup summaries (no task detail).
func (s *Service) Projects() []ProjectRollup {
	projects := s.repo.Projects()
	out := make([]ProjectRollup, len(projects))
	for i, p := range projects {
		out[i] = rollupProject(p, false)
	}
	return out
}

// Project returns one project's full deliverable tree with rollups.
func (s *Service) Project(id string) (ProjectDetail, error) {
	p, ok := s.repo.Project(id)
	if !ok {
		return ProjectDetail{}, ErrNotFound
	}
	return ProjectDetail{
		ProjectRollup: rollupProject(p, true),
		Tasks:         p.Tasks,
		Bloks:         s.repo.BloksByProject(id),
		Kavling:       s.repo.KavlingByProject(id),
	}, nil
}

// AddProjectInput carries the fields required to register a new project. The
// deliverable tree is built from the chosen number of Site Plans and which
// categories to include (see domain.ProjectSpec); it can be edited afterwards.
type AddProjectInput struct {
	GP             string `json:"gp"`
	Name           string `json:"name"`
	Lokasi         string `json:"lokasi"`
	Luas           string `json:"luas"`
	Units          int    `json:"units"`
	Types          int    `json:"types"`
	SitePlans      int    `json:"sitePlans"`
	IncludeUnit    bool   `json:"includeUnit"`
	IncludeKawasan bool   `json:"includeKawasan"`
}

// AddProject registers a new project (instantiating the deliverable tree from
// the spec). Only CEO / Kadep may add projects.
func (s *Service) AddProject(role string, in AddProjectInput) (ProjectDetail, error) {
	if !canManage(role) {
		return ProjectDetail{}, ErrForbidden
	}
	if strings.TrimSpace(in.Name) == "" {
		return ProjectDetail{}, ErrValidation
	}
	gp := strings.TrimSpace(in.GP)
	if gp == "" {
		gp = "GP"
	}
	spec := domain.ProjectSpec{
		SitePlans:      in.SitePlans,
		IncludeUnit:    in.IncludeUnit,
		IncludeKawasan: in.IncludeKawasan,
	}
	p := s.repo.AddProject(gp, strings.TrimSpace(in.Name), strings.TrimSpace(in.Lokasi),
		strings.TrimSpace(in.Luas), in.Units, in.Types, spec)
	return ProjectDetail{ProjectRollup: rollupProject(p, true), Tasks: p.Tasks}, nil
}

// DeleteProject removes a project and everything it owns: its deliverable tasks,
// bloks, kavling, and the review/annotated doc bytes (via the repo cascade), plus
// its task attachment files on disk. Irreversible. CEO / Kadep only.
func (s *Service) DeleteProject(role, id string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	p, ok := s.repo.Project(id)
	if !ok {
		return ErrNotFound
	}
	// Collect task attachment file ids to remove from disk after the state delete.
	var files []string
	for _, t := range p.Tasks {
		for _, a := range t.Attachments {
			files = append(files, a.ID)
		}
	}
	if !s.repo.DeleteProject(id) {
		return ErrNotFound
	}
	s.removeBoardFiles(files) // best-effort disk cleanup (shared upload dir)
	return nil
}

// ProjectImportRow is one parsed spreadsheet row for a bulk project import.
type ProjectImportRow struct {
	Name   string `json:"name"`
	GP     string `json:"gp"`
	Luas   string `json:"luas"`
	Lokasi string `json:"lokasi"`
}

// ProjectImportResult summarizes a bulk project import. Projects are create-only
// (no update path that preserves the task tree), so existing names are skipped.
type ProjectImportResult struct {
	Created    int                `json:"created"`
	Updated    int                `json:"updated"` // always 0 (kept for FE parity)
	GPsCreated []string           `json:"gpsCreated"`
	Skipped    []MasterImportSkip `json:"skipped"`
}

// ImportProjects bulk-creates projects from parsed rows, applying ONE shared
// deliverable template (sitePlans / includeUnit / includeKawasan) to all of them.
// Missing GP masters are auto-created (so the picker stays clean). When
// skipExisting is true a row whose Name already exists is skipped (projects have
// no safe in-place update — it would rebuild the task tree). CEO / Kadep only.
func (s *Service) ImportProjects(role string, rows []ProjectImportRow, sitePlans int, includeUnit, includeKawasan, skipExisting bool) (ProjectImportResult, error) {
	if !canManage(role) {
		return ProjectImportResult{}, ErrForbidden
	}
	res := ProjectImportResult{GPsCreated: []string{}, Skipped: []MasterImportSkip{}}
	eq := func(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

	for i, r := range rows {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			res.Skipped = append(res.Skipped, MasterImportSkip{Row: i + 1, Reason: "nama proyek kosong"})
			continue
		}
		if skipExisting {
			dup := false
			for _, p := range s.repo.Projects() {
				if eq(p.Name, name) {
					dup = true
					break
				}
			}
			if dup {
				res.Skipped = append(res.Skipped, MasterImportSkip{Row: i + 1, Key: name, Reason: "nama proyek sudah ada"})
				continue
			}
		}
		// Auto-create referenced masters so the project's GP/Lokasi resolve.
		gp := strings.TrimSpace(r.GP)
		if gp != "" {
			found := false
			for _, g := range s.repo.GPs() {
				if eq(g.Code, gp) {
					found = true
					break
				}
			}
			if !found {
				s.repo.SaveGP(domain.GP{Code: gp})
				res.GPsCreated = append(res.GPsCreated, gp)
			}
		}
		lokasi := strings.TrimSpace(r.Lokasi)
		if _, err := s.AddProject(role, AddProjectInput{
			GP: gp, Name: name, Lokasi: lokasi, Luas: strings.TrimSpace(r.Luas),
			SitePlans: sitePlans, IncludeUnit: includeUnit, IncludeKawasan: includeKawasan,
		}); err != nil {
			res.Skipped = append(res.Skipped, MasterImportSkip{Row: i + 1, Key: name, Reason: "gagal dibuat"})
			continue
		}
		res.Created++
	}
	return res, nil
}

// AddTaskInput is a new deliverable added to a project's tree.
type AddTaskInput struct {
	Category string          `json:"category"`
	Group    string          `json:"group"`
	Name     string          `json:"name"`
	PIC      string          `json:"pic"`
	Output   domain.Division `json:"output"`
	Weighted bool            `json:"weighted"`
}

// AddTask adds a deliverable to a project (dynamic structure editing). Only
// CEO / Kadep may edit structure.
func (s *Service) AddTask(role, projectID string, in AddTaskInput) (ProjectDetail, error) {
	if !canManage(role) {
		return ProjectDetail{}, ErrForbidden
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Category) == "" ||
		strings.TrimSpace(in.Group) == "" {
		return ProjectDetail{}, ErrValidation
	}
	if !s.validPIC(in.PIC) || !s.validDivision(in.Output) {
		return ProjectDetail{}, ErrValidation
	}
	_, ok := s.repo.AddTask(projectID, domain.Task{
		Category: strings.TrimSpace(in.Category), Group: strings.TrimSpace(in.Group),
		Name: strings.TrimSpace(in.Name), PIC: in.PIC, Output: in.Output, Weighted: in.Weighted,
	})
	if !ok {
		return ProjectDetail{}, ErrNotFound
	}
	return s.Project(projectID)
}

// RemoveTask deletes a deliverable from a project. Only CEO / Kadep.
func (s *Service) RemoveTask(role, projectID, taskID string) (ProjectDetail, error) {
	if !canManage(role) {
		return ProjectDetail{}, ErrForbidden
	}
	if !s.repo.RemoveTask(projectID, taskID) {
		return ProjectDetail{}, ErrNotFound
	}
	return s.Project(projectID)
}

// ReassignTaskInput changes a task's PIC and/or output division.
type ReassignTaskInput struct {
	PIC    string          `json:"pic"`
	Output domain.Division `json:"output"`
}

// ReassignTask changes who owns a task and where its output is routed (dynamic
// assignment). Only CEO / Kadep.
func (s *Service) ReassignTask(role, projectID, taskID string, in ReassignTaskInput) (ProjectDetail, error) {
	if !canManage(role) {
		return ProjectDetail{}, ErrForbidden
	}
	if !s.validPIC(in.PIC) || !s.validDivision(in.Output) {
		return ProjectDetail{}, ErrValidation
	}
	if !s.repo.UpdateTaskMeta(projectID, taskID, in.PIC, in.Output) {
		return ProjectDetail{}, ErrNotFound
	}
	return s.Project(projectID)
}

/* ---- Review flow: upload PDF -> Kadep approves (-> Selesai) ------------- */

// maxDocBytes caps an uploaded review PDF (100 MiB — real CAD-exported
// deliverable/gambar-kerja PDFs can be very large).
const maxDocBytes = 100 << 20

// canApprove reports whether a role may approve/reject/revise a review
// (Kadep, CEO, or the operational director Dirops).
func canApprove(role string) bool {
	return role == domain.RoleKadep || role == domain.RoleCEO || role == domain.RoleDirops
}

// taskPIC finds the owning PIC of a task (and whether the task exists).
func (s *Service) taskPIC(projectID, taskID string) (string, bool) {
	p, ok := s.repo.Project(projectID)
	if !ok {
		return "", false
	}
	for _, t := range p.Tasks {
		if t.ID == taskID {
			return t.PIC, true
		}
	}
	return "", false
}

// UploadTaskDoc stores a PDF for a task and moves it to Review. Permitted for
// the owning PIC or a manager. The PDF must be a non-empty .pdf within the size
// cap.
func (s *Service) UploadTaskDoc(actor domain.User, projectID, taskID, filename string, data []byte) (ProjectDetail, error) {
	pic, ok := s.taskPIC(projectID, taskID)
	if !ok {
		return ProjectDetail{}, ErrNotFound
	}
	if !canManage(actor.Role) && actor.Username != pic {
		return ProjectDetail{}, ErrForbidden
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return ProjectDetail{}, fmt.Errorf("%w: hanya file PDF yang diperbolehkan", ErrValidation)
	}
	if len(data) == 0 {
		return ProjectDetail{}, fmt.Errorf("%w: file kosong", ErrValidation)
	}
	if len(data) > maxDocBytes {
		return ProjectDetail{}, fmt.Errorf("%w: ukuran PDF %d MB melebihi batas %d MB", ErrValidation, len(data)>>20, maxDocBytes>>20)
	}
	doc := domain.TaskDoc{
		Name: filename, Size: len(data),
		UploadedBy: actor.Username, UploadedAt: s.now().Format(time.RFC3339),
	}
	if !s.repo.SetTaskDoc(projectID, taskID, doc, data) {
		return ProjectDetail{}, ErrNotFound
	}
	// Uploading a deliverable puts it up for review (and clears any prior approval
	// + revision instruction, since the re-upload is the response to it).
	s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.Status = domain.StatusReview
		t.ApprovedBy, t.ApprovedAt = "", ""
		t.RevisiNote = ""
		t.UpdatedAt = s.now().Format(time.RFC3339)
	})
	return s.Project(projectID)
}

// TaskDoc returns a task's stored review PDF bytes and filename.
func (s *Service) TaskDoc(projectID, taskID string) ([]byte, string, error) {
	data, name, ok := s.repo.TaskDocBytes(projectID, taskID)
	if !ok {
		return nil, "", ErrNotFound
	}
	return data, name, nil
}

// ApproveTask approves a task's review document. Only Kadep / CEO. On approval
// the task is automatically completed (Selesai). A document must be present.
func (s *Service) ApproveTask(actor domain.User, projectID, taskID string) (ProjectDetail, error) {
	if !canApprove(actor.Role) {
		return ProjectDetail{}, ErrForbidden
	}
	var hasDoc bool
	ok := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		if t.Doc == nil {
			return
		}
		hasDoc = true
		t.Status = domain.StatusDone
		t.ApprovedBy = actor.Username
		t.ApprovedAt = s.now().Format(time.RFC3339)
		t.UpdatedAt = t.ApprovedAt
		t.RevisiNote = "" // approval clears any prior revision instruction
	})
	if !ok {
		return ProjectDetail{}, ErrNotFound
	}
	if !hasDoc {
		return ProjectDetail{}, ErrValidation
	}
	return s.Project(projectID)
}

// RejectTask sends a task's review back to Proses (revision needed). Only Kadep
// / CEO. `instruction` is an optional revision note recorded on the task so the
// owning PIC sees what to fix (empty for a plain reject/"Tolak").
func (s *Service) RejectTask(actor domain.User, projectID, taskID, instruction string) (ProjectDetail, error) {
	if !canApprove(actor.Role) {
		return ProjectDetail{}, ErrForbidden
	}
	ok := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.Status = domain.StatusProgress
		t.ApprovedBy, t.ApprovedAt = "", ""
		t.RevisiNote = instruction
		t.UpdatedAt = s.now().Format(time.RFC3339)
	})
	if !ok {
		return ProjectDetail{}, ErrNotFound
	}
	return s.Project(projectID)
}

// validPIC reports whether the username is a known account.
func (s *Service) validPIC(username string) bool {
	_, ok := s.repo.UserByUsername(username)
	return ok
}

// validDivision reports whether d is an acceptable output target. The output is
// a free-form department code now (dynamic, from the central catalogue) and the
// frontend only offers valid departments, so we accept any value: "" (no
// division), a current department code, OR a legacy code (e.g. "legal" from
// before divisions went dynamic) — the latter is preserved as-is and simply
// dropped from OutputsByDivision aggregation until re-picked. Blocking a task
// edit just because its OLD output no longer maps to a department is worse.
func (s *Service) validDivision(_ domain.Division) bool { return true }

// UpdateTask changes a task's status. Permitted for CEO / Kadep, or the author
// (PIC) who owns the task.
func (s *Service) UpdateTask(actor domain.User, projectID, taskID string, status domain.TaskStatus) error {
	if !validStatus(status) {
		return ErrValidation
	}
	// Peek the owning PIC first to enforce permissions before mutating.
	p, ok := s.repo.Project(projectID)
	if !ok {
		return ErrNotFound
	}
	pic, found := "", false
	for _, t := range p.Tasks {
		if t.ID == taskID {
			pic, found = t.PIC, true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	if !canManage(actor.Role) && actor.Username != pic {
		return ErrForbidden
	}
	if _, ok := s.repo.UpdateTaskStatus(projectID, taskID, status, s.now().Format(time.RFC3339)); !ok {
		return ErrNotFound
	}
	return nil
}

func validStatus(st domain.TaskStatus) bool {
	switch st {
	case domain.StatusTodo, domain.StatusProgress, domain.StatusReview, domain.StatusDone:
		return true
	}
	return false
}

// SetTaskSchedule persists a task's planning dates (Mulai/Deadline/Selesai,
// YYYY-MM-DD, or "" to clear). Each field is a pointer — nil = leave unchanged.
// Editable by the owning PIC or a manager (same as status).
func (s *Service) SetTaskSchedule(actor domain.User, projectID, taskID string, start, deadline, finish *string) error {
	p, ok := s.repo.Project(projectID)
	if !ok {
		return ErrNotFound
	}
	pic, found := "", false
	for _, t := range p.Tasks {
		if t.ID == taskID {
			pic, found = t.PIC, true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	if !canManage(actor.Role) && actor.Username != pic {
		return ErrForbidden
	}
	if !s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		if start != nil {
			t.Start = strings.TrimSpace(*start)
		}
		if deadline != nil {
			t.Deadline = strings.TrimSpace(*deadline)
		}
		if finish != nil {
			t.Finish = strings.TrimSpace(*finish)
		}
	}) {
		return ErrNotFound
	}
	return nil
}

// TaskImportRow is one parsed row for a project task backfill (match by name).
type TaskImportRow struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Start    string `json:"start"`
	Deadline string `json:"deadline"`
	Finish   string `json:"finish"`
}

// TaskImportResult summarizes a task backfill import.
type TaskImportResult struct {
	Updated int                `json:"updated"`
	Skipped []MasterImportSkip `json:"skipped"`
}

// ImportTasks backfills EXISTING project tasks (matched by name, case-insensitive)
// with a status + planning dates. Rows whose name matches no task are skipped +
// reported (tasks are defined in the Deliverable editor, not created here). A
// blank status/date leaves that field unchanged. CEO / Kadep only.
func (s *Service) ImportTasks(role, projectID string, rows []TaskImportRow) (TaskImportResult, error) {
	if !canManage(role) {
		return TaskImportResult{}, ErrForbidden
	}
	p, ok := s.repo.Project(projectID)
	if !ok {
		return TaskImportResult{}, ErrNotFound
	}
	res := TaskImportResult{Skipped: []MasterImportSkip{}}
	byName := map[string]string{} // lower(name) -> taskID
	for _, t := range p.Tasks {
		byName[strings.ToLower(strings.TrimSpace(t.Name))] = t.ID
	}
	now := s.now().Format(time.RFC3339)
	for i, r := range rows {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			res.Skipped = append(res.Skipped, MasterImportSkip{Row: i + 1, Reason: "nama task kosong"})
			continue
		}
		tid, found := byName[strings.ToLower(name)]
		if !found {
			res.Skipped = append(res.Skipped, MasterImportSkip{Row: i + 1, Key: name, Reason: "task tidak ditemukan di proyek"})
			continue
		}
		st := normalizeImportStatus(r.Status)
		s.repo.MutateTask(projectID, tid, func(t *domain.Task) {
			if st != "" {
				t.Status = st
				t.UpdatedAt = now
			}
			if v := strings.TrimSpace(r.Start); v != "" {
				t.Start = v
			}
			if v := strings.TrimSpace(r.Deadline); v != "" {
				t.Deadline = v
			}
			if v := strings.TrimSpace(r.Finish); v != "" {
				t.Finish = v
			}
		})
		res.Updated++
	}
	return res, nil
}

// normalizeImportStatus maps a free-form status/progress cell to a TaskStatus
// ("" = leave the task's status unchanged).
func normalizeImportStatus(v string) domain.TaskStatus {
	n := strings.ToLower(strings.TrimSpace(v))
	if n == "" {
		return ""
	}
	switch {
	case strings.Contains(n, "selesai") || strings.Contains(n, "done") || strings.Contains(n, "complete") || strings.Contains(n, "100") || n == "ya" || n == "y" || n == "v" || n == "✓":
		return domain.StatusDone
	case strings.Contains(n, "review") || strings.Contains(n, "cek"):
		return domain.StatusReview
	case strings.Contains(n, "progress") || strings.Contains(n, "proses") || strings.Contains(n, "dikerjakan") || strings.Contains(n, "jalan") || strings.Contains(n, "wip"):
		return domain.StatusProgress
	case strings.Contains(n, "todo") || strings.Contains(n, "belum") || n == "-":
		return domain.StatusTodo
	}
	return ""
}

/* ---- Task assignment (flow membagi tugas: by PIC account) -------------- */

// TasksForPIC returns every task assigned to the given PIC across all projects,
// each annotated with its project. Backs the "Tugas Saya" view.
func (s *Service) TasksForPIC(pic string) []AssignedTask {
	out := []AssignedTask{}
	for _, p := range s.repo.Projects() {
		for _, t := range p.Tasks {
			if t.PIC == pic {
				out = append(out, AssignedTask{
					ProjectID: p.ID, ProjectName: p.Name, GP: p.GP, Task: t,
				})
			}
		}
	}
	return out
}
