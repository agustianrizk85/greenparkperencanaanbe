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
	RoleDirops  = "dirops"  // operational director (cross-division) — may approve & manage
	RoleKadep   = "kadep"   // head of department, manages projects & assignments
	RoleArsitek = "arsitek" // author of design + render deliverables (Randi, Ananto)
	RoleDrafter = "drafter" // author of working drawings / gambar kerja (Agus, Rio)
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

// TaskDoc is metadata of the PDF a PIC uploads when a task enters Review. The
// bytes are stored separately in the repository (not serialised in the Task).
type TaskDoc struct {
	Name       string `json:"name"`       // original filename
	Size       int    `json:"size"`       // bytes
	UploadedBy string `json:"uploadedBy"` // PIC username
	UploadedAt string `json:"uploadedAt"` // RFC3339
}

// Task is a per-project instance of a deliverable in the planning tree.
//
//	Category -> Group -> Task (leaf, owned by one PIC, routed to one Division)
//
// Review flow: the PIC moves a task to Review and uploads a PDF (Doc); the head
// of department (Kadep) then approves it (-> Selesai, recording ApprovedBy/At)
// or rejects it (-> back to Proses).
type Task struct {
	ID         string     `json:"id"`        // stable slug, unique within a project
	Category   string     `json:"category"`  // "Site Plan" | "Desain Unit Hunian" | "Desain Kawasan"
	Group      string     `json:"group"`     // mid-level deliverable, e.g. "Denah", "Interior"
	Name       string     `json:"name"`      // leaf deliverable name
	PIC        string     `json:"pic"`       // username of the responsible author
	Output     Division   `json:"output"`    // routed division (or DivNone)
	Weighted   bool       `json:"weighted"`  // part of a "100%" milestone group
	Status     TaskStatus `json:"status"`    //
	UpdatedAt  string     `json:"updatedAt"` // RFC3339 of last status change ("" if never)
	Doc        *TaskDoc   `json:"doc,omitempty"`        // review document, if uploaded
	ApprovedBy string     `json:"approvedBy,omitempty"` // Kadep username (when approved)
	ApprovedAt string     `json:"approvedAt,omitempty"` // RFC3339 (when approved)
	RevisiNote string     `json:"revisiNote,omitempty"` // revision instruction when sent back (Revisi)
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
	Attachments    []WDAttachment    `json:"attachments,omitempty"` // files (e.g. imported from cicle)

	// Deep Revisi AI — GK Kontraktor vs GK TTD vision check (Ollama). Bytes are
	// stored separately in the repository, mirroring TaskDoc.
	GKKontraktor *GKDoc            `json:"gkKontraktor,omitempty"`
	GKTTD        *GKDoc            `json:"gkTTD,omitempty"`
	GKAnnotated  *GKDoc            `json:"gkAnnotated,omitempty"`
	GKStatus     GKCheckStatus     `json:"gkStatus,omitempty"` // "", idle, running, done, failed
	GKFindings   []GKFinding       `json:"gkFindings,omitempty"`
	GKError      string            `json:"gkError,omitempty"`
	GKCheckedAt  string            `json:"gkCheckedAt,omitempty"` // RFC3339, last completed run
}

// WDAttachment is a file linked to a working drawing (name + download URL).
// Populated e.g. by the cicle sync from a card's attachments.
type WDAttachment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// GKCheckStatus is the async state of a Deep Revisi AI run.
type GKCheckStatus string

const (
	GKIdle    GKCheckStatus = "idle"
	GKRunning GKCheckStatus = "running"
	GKDone    GKCheckStatus = "done"
	GKFailed  GKCheckStatus = "failed"
)

// GKDoc is metadata of an uploaded/generated Gambar Kerja PDF (kontraktor, ttd,
// or the annotated output). Bytes are stored separately in the repository.
type GKDoc struct {
	Name       string `json:"name"`
	Size       int    `json:"size"`
	UploadedBy string `json:"uploadedBy"`
	UploadedAt string `json:"uploadedAt"` // RFC3339
}

// GKFinding is one inconsistency found by Deep Revisi AI on a given page of
// GK Kontraktor, compared against the corresponding page of GK TTD.
type GKFinding struct {
	Page       int    `json:"page"`       // 1-based page number in GK Kontraktor
	Wrong      string `json:"wrong"`      // value found in GK Kontraktor
	Correct    string `json:"correct"`    // value found in GK TTD
	Explain    string `json:"explain"`    // short explanation
	Confidence string `json:"confidence"` // "tinggi" | "sedang" | "rendah"
}
