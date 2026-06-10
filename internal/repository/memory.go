// Package repository holds the in-memory planning store. The project master
// list is seeded from projects.json (the department's real portfolio); every
// project is expanded into the deliverable task tree, and the per-consumer
// working-drawing flow is stored alongside. User accounts for authentication
// live here too. The store is mutable and guarded by a mutex.
package repository

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"greenpark/perencanaan/internal/domain"
)

// projectsSeed is the embedded master project list (no, gp, name, lokasi, ...).
//
//go:embed projects.json
var projectsSeed []byte

// masterProject mirrors one row of projects.json.
type masterProject struct {
	No     int    `json:"no"`
	GP     string `json:"gp"`
	Name   string `json:"name"`
	Lokasi string `json:"lokasi"`
	Luas   string `json:"luas"`
	Units  int    `json:"units"`
	Types  int    `json:"types"`
}

// Memory is the in-memory store.
type Memory struct {
	mu       sync.RWMutex
	users    map[string]domain.User
	projects map[string]*domain.Project // keyed by project ID
	drawings map[string]*domain.WorkDrawing
	nextNo   int // next project number for additions
	seqWD    int // monotonic counter for work-drawing IDs
}

// NewMemory builds the store, seeding users and the project portfolio (each
// expanded into the deliverable task tree).
func NewMemory() *Memory {
	m := &Memory{
		users:    seedUsers(),
		projects: map[string]*domain.Project{},
		drawings: map[string]*domain.WorkDrawing{},
	}
	m.seedProjects()
	return m
}

func (m *Memory) seedProjects() {
	var rows []masterProject
	if err := json.Unmarshal(projectsSeed, &rows); err != nil {
		panic("seed projects: " + err.Error())
	}
	for _, r := range rows {
		id := fmt.Sprintf("gp-%03d", r.No)
		m.projects[id] = &domain.Project{
			ID:     id,
			No:     r.No,
			GP:     r.GP,
			Name:   r.Name,
			Lokasi: r.Lokasi,
			Luas:   r.Luas,
			Units:  r.Units,
			Types:  r.Types,
			Tasks:  domain.NewTaskTree(),
		}
		if r.No >= m.nextNo {
			m.nextNo = r.No + 1
		}
	}
}

/* ---- Users ------------------------------------------------------------- */

// UserByUsername looks up an account for authentication.
func (m *Memory) UserByUsername(username string) (domain.User, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[username]
	return u, ok
}

/* ---- Projects ---------------------------------------------------------- */

// Projects returns all projects as copies, sorted by number.
func (m *Memory) Projects() []domain.Project {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Project, 0, len(m.projects))
	for _, p := range m.projects {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].No < out[j].No })
	return out
}

// Project returns a copy of one project.
func (m *Memory) Project(id string) (domain.Project, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[id]
	if !ok {
		return domain.Project{}, false
	}
	return *p, true
}

// AddProject creates a new project with a fresh deliverable tree and returns it.
func (m *Memory) AddProject(gp, name, lokasi, luas string, units, types int) domain.Project {
	m.mu.Lock()
	defer m.mu.Unlock()
	no := m.nextNo
	m.nextNo++
	id := fmt.Sprintf("gp-%03d", no)
	p := &domain.Project{
		ID:     id,
		No:     no,
		GP:     gp,
		Name:   name,
		Lokasi: lokasi,
		Luas:   luas,
		Units:  units,
		Types:  types,
		Tasks:  domain.NewTaskTree(),
	}
	m.projects[id] = p
	return *p
}

// UpdateTaskStatus sets the status of a single task. It returns the task's PIC
// (for permission checks done by the caller) and whether the task was found.
func (m *Memory) UpdateTaskStatus(projectID, taskID string, status domain.TaskStatus, at string) (pic string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, found := m.projects[projectID]
	if !found {
		return "", false
	}
	for i := range p.Tasks {
		if p.Tasks[i].ID == taskID {
			p.Tasks[i].Status = status
			p.Tasks[i].UpdatedAt = at
			return p.Tasks[i].PIC, true
		}
	}
	return "", false
}

/* ---- Work drawings ----------------------------------------------------- */

// WorkDrawings returns a copy of all work-drawing flows, newest first.
func (m *Memory) WorkDrawings() []domain.WorkDrawing {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.WorkDrawing, 0, len(m.drawings))
	for _, d := range m.drawings {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// AddWorkDrawing stores a new flow and returns it.
func (m *Memory) AddWorkDrawing(d domain.WorkDrawing) domain.WorkDrawing {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seqWD++
	d.ID = fmt.Sprintf("wd-%04d", m.seqWD)
	stored := d
	m.drawings[d.ID] = &stored
	return stored
}

// MutateWorkDrawing applies fn to a stored drawing under the lock and returns
// the updated copy. fn may read and write the drawing in place.
func (m *Memory) MutateWorkDrawing(id string, fn func(*domain.WorkDrawing)) (domain.WorkDrawing, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drawings[id]
	if !ok {
		return domain.WorkDrawing{}, false
	}
	fn(d)
	return *d, true
}
