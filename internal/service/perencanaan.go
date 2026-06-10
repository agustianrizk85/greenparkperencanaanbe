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
	repo     *repository.Memory
	sessions *auth.SessionStore
	now      func() time.Time // injectable clock (defaults to time.Now)
}

// New builds a Service from the store and session manager.
func New(repo *repository.Memory, sessions *auth.SessionStore) *Service {
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

// AddProjectInput carries the fields required to register a new project.
type AddProjectInput struct {
	GP     string `json:"gp"`
	Name   string `json:"name"`
	Lokasi string `json:"lokasi"`
	Luas   string `json:"luas"`
	Units  int    `json:"units"`
	Types  int    `json:"types"`
}

// AddProject registers a new project (instantiating the deliverable template).
// Only CEO / Kadep may add projects.
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
	p := s.repo.AddProject(gp, strings.TrimSpace(in.Name), strings.TrimSpace(in.Lokasi),
		strings.TrimSpace(in.Luas), in.Units, in.Types)
	return ProjectDetail{ProjectRollup: rollupProject(p, true), Tasks: p.Tasks}, nil
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
