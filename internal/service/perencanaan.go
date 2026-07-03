// Package service holds the business logic of the Perencanaan (planning) module:
// the project deliverable tree, task assignment to the three design authors,
// the routing of finished deliverables to downstream divisions, and the
// per-consumer working-drawing (gambar kerja) flow with its SLA alerts.
package service

import (
	"errors"
	"strings"
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
	repo     repository.Store
	sessions *auth.SessionStore
	now      func() time.Time // injectable clock (defaults to time.Now)
}

// New builds a Service from the store and session manager. repo may be the
// in-memory store or the Postgres-backed one — both satisfy repository.Store.
func New(repo repository.Store, sessions *auth.SessionStore) *Service {
	return &Service{repo: repo, sessions: sessions, now: time.Now}
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
	return role == domain.RoleCEO || role == domain.RoleKadep
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
	if !s.validPIC(in.PIC) || !validDivision(in.Output) {
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
	if !s.validPIC(in.PIC) || !validDivision(in.Output) {
		return ProjectDetail{}, ErrValidation
	}
	if !s.repo.UpdateTaskMeta(projectID, taskID, in.PIC, in.Output) {
		return ProjectDetail{}, ErrNotFound
	}
	return s.Project(projectID)
}

/* ---- Review flow: upload PDF -> Kadep approves (-> Selesai) ------------- */

// maxDocBytes caps an uploaded review PDF (10 MiB).
const maxDocBytes = 10 << 20

// canApprove reports whether a role may approve/reject a review (Kadep or CEO).
func canApprove(role string) bool {
	return role == domain.RoleKadep || role == domain.RoleCEO
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
	if len(data) == 0 || len(data) > maxDocBytes || !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return ProjectDetail{}, ErrValidation
	}
	doc := domain.TaskDoc{
		Name: filename, Size: len(data),
		UploadedBy: actor.Username, UploadedAt: s.now().Format(time.RFC3339),
	}
	if !s.repo.SetTaskDoc(projectID, taskID, doc, data) {
		return ProjectDetail{}, ErrNotFound
	}
	// Uploading a deliverable puts it up for review (and clears any prior approval).
	s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.Status = domain.StatusReview
		t.ApprovedBy, t.ApprovedAt = "", ""
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
// / CEO.
func (s *Service) RejectTask(actor domain.User, projectID, taskID string) (ProjectDetail, error) {
	if !canApprove(actor.Role) {
		return ProjectDetail{}, ErrForbidden
	}
	ok := s.repo.MutateTask(projectID, taskID, func(t *domain.Task) {
		t.Status = domain.StatusProgress
		t.ApprovedBy, t.ApprovedAt = "", ""
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

// validDivision reports whether d is an acceptable output target for a task.
func validDivision(d domain.Division) bool {
	switch d {
	case domain.DivNone, domain.DivLegal, domain.DivMarketing, domain.DivTeknik, domain.DivKonsumen:
		return true
	}
	return false
}

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
