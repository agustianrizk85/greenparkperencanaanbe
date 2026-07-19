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
	mu          sync.RWMutex
	users       map[string]domain.User
	departments []domain.Department       // central catalogue, synced from auth SSO
	gps         []domain.GP               // grup master
	types       []domain.BuildingType     // tipe bangunan master
	lebars      []domain.Lebar            // lebar kavling master
	lokasis     []domain.Lokasi           // lokasi master
	bloks       []domain.Blok             // per-project phase/cluster master
	kavling     []domain.Kavling          // per-project units
	seqGP       int                       // id counter for GPs
	seqType     int                       // id counter for building types
	seqLebar    int
	seqLokasi   int
	seqBlok     int // id counter for bloks
	seqKav      int // id counter for kavling
	projects    map[string]*domain.Project // keyed by project ID
	drawings    map[string]*domain.WorkDrawing
	docs       map[string][]byte  // review PDF bytes, keyed by projectID + "/" + taskID
	cicleBoard json.RawMessage    // raw mirror of the cicle Kanban board (columns+cards)
	nextNo     int // next project number for additions
	seqWD      int // monotonic counter for work-drawing IDs
	seqTask    int // monotonic counter for dynamically added task IDs
}

// CicleBoard returns the stored raw cicle-board mirror (nil if never synced).
func (m *Memory) CicleBoard() json.RawMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cicleBoard
}

// SetCicleBoard replaces the stored cicle-board mirror.
func (m *Memory) SetCicleBoard(data json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cicleBoard = data
}

// NewMemory builds the store, seeding users and the project portfolio (each
// expanded into the deliverable task tree).
// NewMemory builds a store seeded with the built-in master data (example
// projects + department accounts). Kept for callers/tests that want the demo.
func NewMemory() *Memory { return newMemory(true) }

// newMemory builds the store, optionally seeding the built-in master data. With
// seedMaster=false it starts EMPTY (no example projects, no seed accounts) so the
// deployment holds only real projects + the SSO-synced roster.
func newMemory(seedMaster bool) *Memory {
	m := &Memory{
		users:    map[string]domain.User{},
		projects: map[string]*domain.Project{},
		drawings: map[string]*domain.WorkDrawing{},
		docs:     map[string][]byte{},
	}
	if seedMaster {
		m.users = seedUsers()
		m.seedProjects()
	}
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

// gkDocKey is the map key for a work-drawing's Deep Revisi AI document bytes
// (kind = "kontraktor" | "ttd" | "annotated"). Shares the same m.docs map as
// task review PDFs (distinct "wd/" prefix avoids any collision), so it is
// cleared by ResetProses/ResetMaster and round-trips through SnapshotJSON for
// free, same as task docs.
func gkDocKey(wdID, kind string) string { return "wd/" + wdID + "/" + kind }

// SetWorkDrawingDoc stores a Deep Revisi AI PDF (kontraktor/ttd/annotated) for
// a work drawing and records its metadata on the drawing itself.
func (m *Memory) SetWorkDrawingDoc(wdID, kind string, doc domain.GKDoc, data []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.drawings[wdID]
	if !ok {
		return false
	}
	stored := doc
	switch kind {
	case "kontraktor":
		d.GKKontraktor = &stored
	case "ttd":
		d.GKTTD = &stored
	case "annotated":
		d.GKAnnotated = &stored
	default:
		return false
	}
	m.docs[gkDocKey(wdID, kind)] = data
	return true
}

// WorkDrawingDocBytes returns the stored bytes and filename for a work
// drawing's Deep Revisi AI document (kind = "kontraktor"|"ttd"|"annotated").
func (m *Memory) WorkDrawingDocBytes(wdID, kind string) ([]byte, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.docs[gkDocKey(wdID, kind)]
	if !ok {
		return nil, "", false
	}
	name := wdID
	if d, ok := m.drawings[wdID]; ok {
		var doc *domain.GKDoc
		switch kind {
		case "kontraktor":
			doc = d.GKKontraktor
		case "ttd":
			doc = d.GKTTD
		case "annotated":
			doc = d.GKAnnotated
		}
		if doc != nil {
			name = doc.Name
		}
	}
	return data, name, true
}

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

// SetTaskAIAnnotated stores the Deep Analisis annotated result PDF for a task
// (separate key from the review Doc) and records its metadata on the task.
func (m *Memory) SetTaskAIAnnotated(projectID, taskID, name string, data []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return false
	}
	for i := range p.Tasks {
		if p.Tasks[i].ID == taskID {
			m.docs[docKey(projectID, taskID)+"/annotated"] = data
			p.Tasks[i].AIAnnotated = &domain.TaskDoc{Name: name, Size: len(data), UploadedBy: "deep-analisis-ai"}
			return true
		}
	}
	return false
}

// TaskAIAnnotatedBytes returns the annotated result PDF bytes + filename.
func (m *Memory) TaskAIAnnotatedBytes(projectID, taskID string) ([]byte, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.docs[docKey(projectID, taskID)+"/annotated"]
	if !ok {
		return nil, "", false
	}
	name := "hasil-" + taskID + ".pdf"
	if p, ok := m.projects[projectID]; ok {
		for _, t := range p.Tasks {
			if t.ID == taskID && t.AIAnnotated != nil {
				name = t.AIAnnotated.Name
			}
		}
	}
	return data, name, true
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

// AddUser inserts or replaces an account. Returns false if the username already
// exists (callers decide whether that is an error).
func (m *Memory) AddUser(u domain.User) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.users[u.Username]; exists {
		return false
	}
	m.users[u.Username] = u
	return true
}

// Departments returns the cached central department catalogue.
func (m *Memory) Departments() []domain.Department {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Department, len(m.departments))
	copy(out, m.departments)
	return out
}

// SetDepartments replaces the cached department catalogue (from the auth sync).
func (m *Memory) SetDepartments(depts []domain.Department) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.departments = depts
}

/* ---- GP master ---- */

func (m *Memory) GPs() []domain.GP {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.GP, len(m.gps))
	copy(out, m.gps)
	return out
}

// SaveGP inserts (empty ID) or updates a GP. Returns the stored record.
func (m *Memory) SaveGP(gp domain.GP) domain.GP {
	m.mu.Lock()
	defer m.mu.Unlock()
	if gp.ID == "" {
		m.seqGP++
		gp.ID = fmt.Sprintf("gp-%d", m.seqGP)
		m.gps = append(m.gps, gp)
		return gp
	}
	for i := range m.gps {
		if m.gps[i].ID == gp.ID {
			m.gps[i] = gp
			return gp
		}
	}
	return domain.GP{}
}

func (m *Memory) DeleteGP(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.gps {
		if m.gps[i].ID == id {
			m.gps = append(m.gps[:i], m.gps[i+1:]...)
			return true
		}
	}
	return false
}

/* ---- Building type master ---- */

func (m *Memory) BuildingTypes() []domain.BuildingType {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.BuildingType, len(m.types))
	copy(out, m.types)
	return out
}

func (m *Memory) SaveBuildingType(t domain.BuildingType) domain.BuildingType {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t.ID == "" {
		m.seqType++
		t.ID = fmt.Sprintf("type-%d", m.seqType)
		m.types = append(m.types, t)
		return t
	}
	for i := range m.types {
		if m.types[i].ID == t.ID {
			m.types[i] = t
			return t
		}
	}
	return domain.BuildingType{}
}

func (m *Memory) DeleteBuildingType(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.types {
		if m.types[i].ID == id {
			m.types = append(m.types[:i], m.types[i+1:]...)
			return true
		}
	}
	return false
}

/* ---- Lebar master ---- */

func (m *Memory) Lebars() []domain.Lebar {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Lebar, len(m.lebars))
	copy(out, m.lebars)
	return out
}

func (m *Memory) SaveLebar(l domain.Lebar) domain.Lebar {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l.ID == "" {
		m.seqLebar++
		l.ID = fmt.Sprintf("lebar-%d", m.seqLebar)
		m.lebars = append(m.lebars, l)
		return l
	}
	for i := range m.lebars {
		if m.lebars[i].ID == l.ID {
			m.lebars[i] = l
			return l
		}
	}
	return domain.Lebar{}
}

func (m *Memory) DeleteLebar(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.lebars {
		if m.lebars[i].ID == id {
			m.lebars = append(m.lebars[:i], m.lebars[i+1:]...)
			return true
		}
	}
	return false
}

/* ---- Lokasi master ---- */

func (m *Memory) Lokasis() []domain.Lokasi {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Lokasi, len(m.lokasis))
	copy(out, m.lokasis)
	return out
}

func (m *Memory) SaveLokasi(l domain.Lokasi) domain.Lokasi {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l.ID == "" {
		m.seqLokasi++
		l.ID = fmt.Sprintf("lokasi-%d", m.seqLokasi)
		m.lokasis = append(m.lokasis, l)
		return l
	}
	for i := range m.lokasis {
		if m.lokasis[i].ID == l.ID {
			m.lokasis[i] = l
			return l
		}
	}
	return domain.Lokasi{}
}

func (m *Memory) DeleteLokasi(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.lokasis {
		if m.lokasis[i].ID == id {
			m.lokasis = append(m.lokasis[:i], m.lokasis[i+1:]...)
			return true
		}
	}
	return false
}

/* ---- Blok master (per project) ---- */

func (m *Memory) BloksByProject(projectID string) []domain.Blok {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Blok{}
	for _, b := range m.bloks {
		if b.ProjectID == projectID {
			out = append(out, b)
		}
	}
	return out
}

func (m *Memory) SaveBlok(b domain.Blok) domain.Blok {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b.ID == "" {
		m.seqBlok++
		b.ID = fmt.Sprintf("blok-%d", m.seqBlok)
		m.bloks = append(m.bloks, b)
		return b
	}
	for i := range m.bloks {
		if m.bloks[i].ID == b.ID {
			b.ProjectID = m.bloks[i].ProjectID // project is immutable
			m.bloks[i] = b
			return b
		}
	}
	return domain.Blok{}
}

func (m *Memory) DeleteBlok(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.bloks {
		if m.bloks[i].ID == id {
			m.bloks = append(m.bloks[:i], m.bloks[i+1:]...)
			// Orphan any kavling that referenced this blok.
			for j := range m.kavling {
				if m.kavling[j].BlokID == id {
					m.kavling[j].BlokID = ""
				}
			}
			return true
		}
	}
	return false
}

/* ---- Kavling (per project) ---- */

func (m *Memory) KavlingByProject(projectID string) []domain.Kavling {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []domain.Kavling{}
	for _, k := range m.kavling {
		if k.ProjectID == projectID {
			out = append(out, k)
		}
	}
	return out
}

func (m *Memory) SaveKavling(k domain.Kavling) domain.Kavling {
	m.mu.Lock()
	defer m.mu.Unlock()
	if k.ID == "" {
		m.seqKav++
		k.ID = fmt.Sprintf("kav-%d", m.seqKav)
		m.kavling = append(m.kavling, k)
		return k
	}
	for i := range m.kavling {
		if m.kavling[i].ID == k.ID {
			k.ProjectID = m.kavling[i].ProjectID // project is immutable
			m.kavling[i] = k
			return k
		}
	}
	return domain.Kavling{}
}

func (m *Memory) DeleteKavling(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.kavling {
		if m.kavling[i].ID == id {
			m.kavling = append(m.kavling[:i], m.kavling[i+1:]...)
			return true
		}
	}
	return false
}

// UpsertUser inserts a user, or updates the Name/Role of an existing one while
// preserving its credentials (Salt/PasswordHash). Used by the auth SSO roster
// sync so central account changes reflect here. Returns true if anything changed.
func (m *Memory) UpsertUser(u domain.User) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.users[u.Username]; ok {
		if existing.Name == u.Name && existing.Role == u.Role {
			return false
		}
		existing.Name = u.Name
		existing.Role = u.Role
		m.users[u.Username] = existing
		return true
	}
	m.users[u.Username] = u
	return true
}

// DeleteUser removes an account by username. Returns false if it did not exist.
func (m *Memory) DeleteUser(username string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[username]; !ok {
		return false
	}
	delete(m.users, username)
	return true
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

/* ---- Snapshot persistence (used by the Postgres-backed store) ---------- */

// userSnap carries the password material that domain.User omits from JSON, so a
// snapshot can round-trip accounts (including runtime-added PICs).
type userSnap struct {
	Username     string `json:"username"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Salt         []byte `json:"salt"`
	PasswordHash []byte `json:"passwordHash"`
}

type stateSnap struct {
	Users       []userSnap                     `json:"users"`
	Departments []domain.Department            `json:"departments,omitempty"`
	GPs         []domain.GP                    `json:"gps,omitempty"`
	Types       []domain.BuildingType          `json:"types,omitempty"`
	Lebars      []domain.Lebar                 `json:"lebars,omitempty"`
	Lokasis     []domain.Lokasi                `json:"lokasis,omitempty"`
	Bloks       []domain.Blok                  `json:"bloks,omitempty"`
	Kavling     []domain.Kavling               `json:"kavling,omitempty"`
	Projects    map[string]*domain.Project     `json:"projects"`
	Drawings    map[string]*domain.WorkDrawing `json:"drawings"`
	Docs        map[string][]byte              `json:"docs"`
	CicleBoard  json.RawMessage                `json:"cicleBoard,omitempty"`
	NextNo      int                            `json:"nextNo"`
	SeqWD       int                            `json:"seqWD"`
	SeqTask     int                            `json:"seqTask"`
	SeqGP       int                            `json:"seqGP"`
	SeqType     int                            `json:"seqType"`
	SeqBlok     int                            `json:"seqBlok"`
	SeqKav      int                            `json:"seqKav"`
	SeqLebar    int                            `json:"seqLebar"`
	SeqLokasi   int                            `json:"seqLokasi"`
}

// SnapshotJSON serialises the entire store (including password material) for
// durable persistence. Safe for concurrent use.
func (m *Memory) SnapshotJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := stateSnap{
		Departments: m.departments, GPs: m.gps, Types: m.types, Lebars: m.lebars, Lokasis: m.lokasis,
		Bloks: m.bloks, Kavling: m.kavling,
		Projects: m.projects, Drawings: m.drawings, Docs: m.docs,
		CicleBoard: m.cicleBoard,
		NextNo:     m.nextNo, SeqWD: m.seqWD, SeqTask: m.seqTask,
		SeqGP: m.seqGP, SeqType: m.seqType, SeqBlok: m.seqBlok, SeqKav: m.seqKav,
		SeqLebar: m.seqLebar, SeqLokasi: m.seqLokasi,
	}
	for _, u := range m.users {
		s.Users = append(s.Users, userSnap{u.Username, u.Name, u.Role, u.Salt, u.PasswordHash})
	}
	return json.Marshal(s)
}

// LoadJSON replaces the store contents from a snapshot produced by SnapshotJSON.
func (m *Memory) LoadJSON(data []byte) error {
	var s stateSnap
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users = map[string]domain.User{}
	for _, u := range s.Users {
		m.users[u.Username] = domain.User{Username: u.Username, Name: u.Name, Role: u.Role, Salt: u.Salt, PasswordHash: u.PasswordHash}
	}
	m.departments = s.Departments
	m.gps = s.GPs
	m.types = s.Types
	m.lebars = s.Lebars
	m.lokasis = s.Lokasis
	m.bloks = s.Bloks
	m.kavling = s.Kavling
	m.seqGP, m.seqType = s.SeqGP, s.SeqType
	m.seqBlok, m.seqKav = s.SeqBlok, s.SeqKav
	m.seqLebar, m.seqLokasi = s.SeqLebar, s.SeqLokasi
	m.projects = s.Projects
	if m.projects == nil {
		m.projects = map[string]*domain.Project{}
	}
	m.drawings = s.Drawings
	if m.drawings == nil {
		m.drawings = map[string]*domain.WorkDrawing{}
	}
	m.docs = s.Docs
	if m.docs == nil {
		m.docs = map[string][]byte{}
	}
	m.cicleBoard = s.CicleBoard
	m.nextNo, m.seqWD, m.seqTask = s.NextNo, s.SeqWD, s.SeqTask
	return nil
}
