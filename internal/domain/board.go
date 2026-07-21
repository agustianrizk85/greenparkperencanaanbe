package domain

// This file holds the shared department Kanban board ("Departemen Perencanaan")
// — a Trello/Cycle-style board with lists, cards, labels, checklists,
// attachments and comments. Attachment BYTES live on disk (uploads dir); only
// metadata is stored here, so the state snapshot stays small.

// BoardLabel is a board-wide label definition; cards reference it by ID.
type BoardLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// BoardChecklistItem is one row of a card checklist.
type BoardChecklistItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Done   bool   `json:"done"`
	DoneAt string `json:"doneAt"` // RFC3339 when Done flipped true, "" otherwise
	Due    string `json:"due"`    // RFC3339 or ""
}

// BoardChecklist is a titled checklist on a card.
type BoardChecklist struct {
	ID    string               `json:"id"`
	Title string               `json:"title"`
	Items []BoardChecklistItem `json:"items"`
}

// BoardAttachment is metadata of a file attached to a card. The bytes live on
// disk at <uploadDir>/<ID> — never inside the state snapshot.
type BoardAttachment struct {
	ID   string `json:"id"`
	Name string `json:"name"` // original filename
	Size int64  `json:"size"` // bytes
	Mime string `json:"mime"`
	By   string `json:"by"` // uploader username
	At   string `json:"at"` // RFC3339
}

// BoardComment is one comment on a card.
type BoardComment struct {
	ID     string `json:"id"`
	Author string `json:"author"` // username
	Text   string `json:"text"`
	At     string `json:"at"` // RFC3339
}

// BoardCard is one card on the department board. Slice order within a list's
// Cards IS the display order.
type BoardCard struct {
	ID          string            `json:"id"`
	ListID      string            `json:"listId"`
	Title       string            `json:"title"`
	Desc        string            `json:"desc"` // Catatan
	Members     []string          `json:"members"`
	Labels      []string          `json:"labels"` // BoardLabel IDs
	Due         string            `json:"due"`    // RFC3339 or ""
	DueDone     bool              `json:"dueDone"`
	Cover       string            `json:"cover"`              // attachment ID or ""
	Division    string            `json:"division,omitempty"` // department code the card belongs to ("" = none)
	Checklists  []BoardChecklist  `json:"checklists"`
	Attachments []BoardAttachment `json:"attachments"`
	Comments    []BoardComment    `json:"comments"`
	CreatedBy   string            `json:"createdBy"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
}

// BoardList is one column of the board. List order in the board slice IS the
// display order.
type BoardList struct {
	ID        string      `json:"id"`
	Title     string      `json:"title"`
	System    bool        `json:"system,omitempty"` // fixed status column — cannot be renamed/moved/deleted
	CreatedBy string      `json:"createdBy"`
	Cards     []BoardCard `json:"cards"`
}

// System board list IDs — the 4 fixed status columns that ALWAYS exist at the
// front of the board (mirroring the formal task lifecycle: todo→progress→
// review→done). They are literal constants so they never collide with the
// bl-N id generator.
const (
	SysListTodo     = "sys-todo"
	SysListProgress = "sys-progress"
	SysListReview   = "sys-review"
	SysListDone     = "sys-done"
)

// SystemBoardList is a fixed status column definition (id + title).
type SystemBoardList struct {
	ID    string
	Title string
}

// SystemBoardLists is the fixed, ordered set of the 4 status columns. Order in
// this slice IS the display order at the front of the board.
var SystemBoardLists = []SystemBoardList{
	{SysListTodo, "To Do"},
	{SysListProgress, "Sedang Dikerjakan"},
	{SysListReview, "Review"},
	{SysListDone, "Selesai"},
}

// IsSystemList reports whether id is one of the 4 fixed status-column ids.
func IsSystemList(id string) bool {
	switch id {
	case SysListTodo, SysListProgress, SysListReview, SysListDone:
		return true
	}
	return false
}

// SysListForStatus maps a formal task status to the system list it belongs in.
// An unknown / empty status falls back to To Do.
func SysListForStatus(st TaskStatus) string {
	switch st {
	case StatusProgress:
		return SysListProgress
	case StatusReview:
		return SysListReview
	case StatusDone:
		return SysListDone
	default:
		return SysListTodo
	}
}
