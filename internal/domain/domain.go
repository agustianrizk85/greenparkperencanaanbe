// Package domain holds the core types of the Perencanaan (planning) module:
// the people who sign in, the projects, the deliverable task tree assigned to
// the three design authors, and the per-consumer working-drawing (gambar kerja)
// flow with its SLA deadlines.
package domain

// User is a dashboard operator who can sign in. The password material is never
// serialised to JSON.
type User struct {
	Username     string `json:"username"`
	Name         string `json:"name"`
	Role         string `json:"role"` // one of the Role* constants
	Salt         []byte `json:"-"`
	PasswordHash []byte `json:"-"`
}

// Roles in the planning department.
const (
	RoleCEO     = "ceo"     // full overview, may do anything
	RoleKadep   = "kadep"   // head of department, manages projects & assignments
	RoleArsitek = "arsitek" // author of design + render deliverables (Randi, Ananto)
	RoleDrafter = "drafter" // author of working drawings / gambar kerja (Agus)
)

// Division is a downstream consumer that a finished deliverable is routed to.
type Division string

const (
	DivNone      Division = ""
	DivLegal     Division = "legal"
	DivMarketing Division = "marketing"
	DivTeknik    Division = "teknik"
	DivKonsumen  Division = "konsumen"
	DivCEO       Division = "ceo"
)

// TaskStatus is the lifecycle state of a single deliverable.
type TaskStatus string

const (
	StatusTodo     TaskStatus = "todo"
	StatusProgress TaskStatus = "progress"
	StatusReview   TaskStatus = "review"
	StatusDone     TaskStatus = "done"
)

// Weight of a status when rolling progress up to a percentage.
func (s TaskStatus) Weight() float64 {
	switch s {
	case StatusProgress:
		return 0.34
	case StatusReview:
		return 0.75
	case StatusDone:
		return 1
	default:
		return 0
	}
}

// Task is a per-project instance of a deliverable in the planning tree.
//
//	Category -> Group -> Task (leaf, owned by one PIC, routed to one Division)
type Task struct {
	ID        string     `json:"id"`        // stable slug, unique within a project
	Category  string     `json:"category"`  // "Site Plan" | "Desain Unit Hunian" | "Desain Kawasan"
	Group     string     `json:"group"`     // mid-level deliverable, e.g. "Denah", "Interior"
	Name      string     `json:"name"`      // leaf deliverable name
	PIC       string     `json:"pic"`       // username of the responsible author
	Output    Division   `json:"output"`    // routed division (or DivNone)
	Weighted  bool       `json:"weighted"`  // part of a "100%" milestone group
	Status    TaskStatus `json:"status"`    //
	UpdatedAt string     `json:"updatedAt"` // RFC3339 of last status change ("" if never)
}

// Project is a development project carrying the full deliverable task list.
type Project struct {
	ID     string `json:"id"`
	No     int    `json:"no"`
	GP     string `json:"gp"`
	Name   string `json:"name"`
	Lokasi string `json:"lokasi"`
	Luas   string `json:"luas"`
	Units  int    `json:"units"`
	Types  int    `json:"types"`
	Tasks  []Task `json:"tasks"`
}

// WorkDrawingStatus is the stage of the per-consumer gambar kerja flow.
type WorkDrawingStatus string

const (
	WDInfo       WorkDrawingStatus = "info"       // consumer info received, drawing not started
	WDKonsumen   WorkDrawingStatus = "konsumen"   // consumer drawing in progress (15-working-day SLA)
	WDTTD        WorkDrawingStatus = "ttd"         // consumer signed off, contractor drawing pending
	WDKontraktor WorkDrawingStatus = "kontraktor" // contractor drawing in progress (5-working-day SLA)
	WDDone       WorkDrawingStatus = "done"        // delivered to teknik
)

// WorkDrawing tracks one consumer's working-drawing flow and the two SLA gates:
//   - consumer drawing due 15 working days after info masuk,
//   - contractor drawing due 5 working days after consumer TTD.
type WorkDrawing struct {
	ID             string            `json:"id"`
	ProjectID      string            `json:"projectId"`
	Konsumen       string            `json:"konsumen"`
	Unit           string            `json:"unit"`
	PIC            string            `json:"pic"` // author handling this drawing
	InfoMasuk      string            `json:"infoMasuk"`      // YYYY-MM-DD
	KonsumenDue    string            `json:"konsumenDue"`    // +15 working days (derived)
	KonsumenDone   string            `json:"konsumenDone"`   // actual date, "" if pending
	TTDKonsumen    string            `json:"ttdKonsumen"`    // consumer signature date, "" if pending
	KontraktorDue  string            `json:"kontraktorDue"`  // +5 working days after TTD (derived)
	KontraktorDone string            `json:"kontraktorDone"` // actual date, "" if pending
	Status         WorkDrawingStatus `json:"status"`
	RevisiNote     string            `json:"revisiNote"` // last AI revision analysis, "" if none
}
