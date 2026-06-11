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
	docs     map[string][]byte // review PDF bytes, keyed by projectID + "/" + taskID
	nextNo   int // next project number for additions
	seqWD    int // monotonic counter for work-drawing IDs
	seqTask  int // monotonic counter for dynamically added task IDs
}

// NewMemory builds the store, seeding users and the project portfolio (each
// expanded into the deliverable task tree).
func NewMemory() *Memory {
	m := &Memory{
		users:    seedUsers(),
		projects: map[string]*domain.Project{},
		drawings: map[string]*domain.WorkDrawing{},
		docs:     map[string][]byte{},
	}
	m.seedProjects()
	return m
}

// ResetProses clears only the dynamic PROCESS data: every task status returns
// to "todo" and all working-drawing flows are removed. The MASTER data — the
// project list (including any project added manually) and the deliverable
// structure — is preserved. Backs the "Reset Proses" admin action.
func (m *Memory) ResetProses() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.projects {
		for i := range p.Tasks {
			p.Tasks[i].Status = domain.StatusTodo
			p.Tasks[i].UpdatedAt = ""
			p.Tasks[i].Doc = nil
			p.Tasks[i].ApprovedBy = ""
			p.Tasks[i].ApprovedAt = ""
		}
	}
	m.drawings = map[string]*domain.WorkDrawing{}
	m.docs = map[string][]byte{}
	m.seqWD = 0
}

// ResetMaster rebuilds the MASTER portfolio back to the seeded 32 projects with
// fresh (all-todo) deliverable trees: every manually added project is dropped
// and all process data is cleared too. Seeded user accounts are kept. Backs the
// "Reset Master" admin action.
func (m *Memory) ResetMaster() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projects = map[string]*domain.Project{}
	m.drawings = map[string]*domain.WorkDrawing{}
	m.docs = map[string][]byte{}
	m.nextNo = 0
	m.seqWD = 0
	m.seedProjects()
}

// docKey is the map key for a task's review document bytes.
func docKey(projectID, taskID string) string { return projectID + "/" + taskID }

// MutateTask applies fn to a task in place under the lock. Returns whether the
// task was found. Used for status, approval and rejection transitions.
func (m *Memory) MutateTask(projectID, taskID string, fn func(*domain.Task)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return false
	}
	for i := range p.Tasks {
		if p.Tasks[i].ID == taskID {
			fn(&p.Tasks[i])
			return true
		}
	}
	return false
}

// SetTaskDoc stores a review PDF for a task and records its metadata.
func (m *Memory) SetTaskDoc(projectID, taskID string, doc domain.TaskDoc, data []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return false
	}
	for i := range p.Tasks {
		if p.Tasks[i].ID == taskID {
			d := doc
			p.Tasks[i].Doc = &d
			m.docs[docKey(projectID, taskID)] = data
			return true
		}
	}
	return false
}

// TaskDocBytes returns the stored PDF bytes and filename for a task.
func (m *Memory) TaskDocBytes(projectID, taskID string) ([]byte, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.docs[docKey(projectID, taskID)]
	if !ok {
		return nil, "", false
	}
	name := taskID
	if p, ok := m.projects[projectID]; ok {
		for _, t := range p.Tasks {
			if t.ID == taskID && t.Doc != nil {
				name = t.Doc.Name
			}
		}
	}
	return data, name, true
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

// Users returns every account, sorted by role precedence then name. The
// password material is omitted from the JSON-serialised copies.
func (m *Memory) Users() []domain.User {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		if ri, rj := roleRank(out[i].Role), roleRank(out[j].Role); ri != rj {
			return ri < rj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func roleRank(role string) int {
	switch role {
	case domain.RoleCEO:
		return 0
	case domain.RoleKadep:
		return 1
	case domain.RoleArsitek:
		return 2
	case domain.RoleDrafter:
		return 3
	default:
		return 4
	}
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

// AddProject creates a new project, building its deliverable tree from the
// given spec (number of site plans + which categories), and returns it.
func (m *Memory) AddProject(gp, name, lokasi, luas string, units, types int, spec domain.ProjectSpec) domain.Project {
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
		Tasks:  domain.BuildTaskTree(spec),
	}
	m.projects[id] = p
	return *p
}

// AddTask appends a new deliverable task to a project (dynamic structure
// editing) and returns it. The ID is generated to be unique within the store.
func (m *Memory) AddTask(projectID string, t domain.Task) (domain.Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return domain.Task{}, false
	}
	m.seqTask++
	t.ID = fmt.Sprintf("task-%d", m.seqTask)
	t.Status = domain.StatusTodo
	p.Tasks = append(p.Tasks, t)
	return t, true
}

// RemoveTask deletes a task from a project.
func (m *Memory) RemoveTask(projectID, taskID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return false
	}
	for i := range p.Tasks {
		if p.Tasks[i].ID == taskID {
			p.Tasks = append(p.Tasks[:i], p.Tasks[i+1:]...)
			return true
		}
	}
	return false
}

// UpdateTaskMeta reassigns a task's PIC and output division (structure edit,
// distinct from status). Returns whether the task was found.
func (m *Memory) UpdateTaskMeta(projectID, taskID, pic string, output domain.Division) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return false
	}
	for i := range p.Tasks {
		if p.Tasks[i].ID == taskID {
			p.Tasks[i].PIC = pic
			p.Tasks[i].Output = output
			return true
		}
	}
	return false
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
