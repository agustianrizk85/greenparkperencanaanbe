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
	Role         string `json:"role"`               // one of the Role* constants
	Division     string `json:"division,omitempty"` // home department code ("" = perencanaan); set for cross-division SSO board users
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

// Department is a division in the central catalogue (synced from auth SSO). A
// finished deliverable's Output routes to one of these by Code.
type Department struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// GP is a grup/cluster master (GP1, GP2, …) — a Project belongs to one GP.
type GP struct {
	ID   string `json:"id"`
	Code string `json:"code"` // GP1
	Name string `json:"name"` // brand / cluster name (optional)
}

// BuildingType is a reusable house-type master (Garnet, Ruby, …) with its
// standard building + land area, referenced by kavling.
type BuildingType struct {
	ID           string `json:"id"`
	Name         string `json:"name"`         // Garnet
	LuasBangunan int    `json:"luasBangunan"` // 42
	LuasTanah    int    `json:"luasTanah"`    // 32
}

// Lebar is a kavling-frontage category master (L3.5, L4, L5) — a controlled
// vocabulary so kavling pick a value instead of free-typing.
type Lebar struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Lokasi is a location master (Leuwinanggung, Curug, …) reused across projects.
type Lokasi struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Blok is a phase/cluster grouping WITHIN a project (A, B, "Verci 3 Ekstensi").
type Blok struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}

// Kavling is one unit/plot in a project: it sits in a Blok and is built to a
// BuildingType, with its actual plot size + frontage.
type Kavling struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	BlokID       string `json:"blokId"`       // → Blok ("" = unassigned)
	NoKav        string `json:"noKav"`        // A1
	TypeID       string `json:"typeId"`       // → BuildingType ("" = unset)
	LuasBangunan int    `json:"luasBangunan"` // actual (usually = type's)
	LuasKavling  int    `json:"luasKavling"`  // actual land plot
	LebarKavling string `json:"lebarKavling"` // L4, L3.5
}

// Division is a downstream consumer that a finished deliverable is routed to.
// It holds a department Code (dynamic, from the central catalogue); the Div*
// constants below are legacy defaults kept for compatibility.
type Division string

const (
	// Default template outputs — values are the CENTRAL department codes (from
	// auth SSO) so a freshly-created project's deliverables route to real
	// departments. Legacy "legal"/"konsumen" values (pre-dynamic-divisions) are
	// still accepted on existing tasks; validDivision no longer blocks them.
	DivNone      Division = ""
	DivLegal     Division = "legalpermit"
	DivMarketing Division = "marketing"
	DivTeknik    Division = "teknik"
	DivKonsumen  Division = "cso"
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

// TaskAttachment is metadata of a file attached to a formal Task (any file type,
// up to 1 GiB). Same shape as BoardAttachment: the bytes live on disk at
// <uploadDir>/<ID> (never in the state snapshot), only the metadata rides in the
// Task (and thus the Project snapshot).
type TaskAttachment struct {
	ID   string `json:"id"`
	Name string `json:"name"` // original filename
	Size int64  `json:"size"` // bytes
	Mime string `json:"mime"`
	By   string `json:"by"` // uploader username
	At   string `json:"at"` // RFC3339
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

	// Attachments are arbitrary files linked to the task (any type, ≤1 GiB each).
	// Bytes live on disk (uploadDir/<attId>); only this metadata is persisted.
	Attachments []TaskAttachment `json:"attachments,omitempty"`

	// Comments is the task's discussion thread (same shape as board-card comments).
	Comments []BoardComment `json:"comments,omitempty"`

	// Deep Analisis AI — single-document vision QC of the review PDF (Doc) against
	// the selected checklist skill(s), producing an annotated result PDF. State is
	// ephemeral, mirroring the WorkDrawing GK block.
	AIStatus    GKCheckStatus `json:"aiStatus,omitempty"`  // "", running, done, failed
	AIDone      int           `json:"aiDone,omitempty"`    // pages analysed so far (progress)
	AITotal     int           `json:"aiTotal,omitempty"`   // total pages to analyse
	AIFindings  []GKFinding   `json:"aiFindings,omitempty"`
	AISkills    []string      `json:"aiSkills,omitempty"`    // skill names applied this run
	AIAnnotated *TaskDoc      `json:"aiAnnotated,omitempty"` // annotated result PDF (bytes stored in repo)
	AIError     string        `json:"aiError,omitempty"`
	AICheckedAt string        `json:"aiCheckedAt,omitempty"` // RFC3339, last completed run

	// Schedule (Proyek view) — server-persisted planning dates, YYYY-MM-DD
	// ("" = unset). Previously localStorage-only; now shared/persistent.
	Start    string `json:"start,omitempty"`
	Deadline string `json:"deadline,omitempty"`
	Finish   string `json:"finish,omitempty"`
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
	GKDone       int               `json:"gkDone,omitempty"`   // pages analysed so far (progress)
	GKTotal      int               `json:"gkTotal,omitempty"`  // total pages to analyse
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
