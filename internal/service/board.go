// Department Kanban board ("Departemen Perencanaan") — Trello/Cycle-style
// lists, cards, labels, checklists, attachments and comments on ONE shared
// board. Permission model:
//
//   - admin        : ceo | dirops | kadep (canManage) — full control.
//   - contributor  : admin OR the card's creator OR a card member — may edit
//     that card (title/desc/due/labels/members/checklists/attachments/move).
//   - viewer       : any other authenticated user — read-only on that card,
//     but may still create lists/cards and add comments.
//
// Attachment BYTES live on disk at <uploadDir>/<attID>; the store snapshot
// holds metadata only.
package service

import (
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"greenpark/perencanaan/internal/domain"
)

// MaxBoardAttachmentBytes caps ONE uploaded board attachment (1 GiB).
const MaxBoardAttachmentBytes int64 = 1 << 30

// UploadDir is where board attachment files live (the transport layer creates
// its streaming temp files there so the final rename stays on one volume).
func (s *Service) UploadDir() string { return s.uploadDir }

// boardContributor reports whether actor may edit the given card.
func boardContributor(actor domain.User, c domain.BoardCard) bool {
	if canManage(actor.Role) || c.CreatedBy == actor.Username {
		return true
	}
	for _, m := range c.Members {
		if m == actor.Username {
			return true
		}
	}
	return false
}

// rfc3339OrEmpty validates a due-style timestamp: RFC3339 or "" (clears).
func rfc3339OrEmpty(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	if _, err := time.Parse(time.RFC3339, v); err != nil {
		return "", fmt.Errorf("%w: tanggal harus berformat RFC3339", ErrValidation)
	}
	return v, nil
}

// nowRFC3339 is the timestamp used for every board mutation.
func (s *Service) nowRFC3339() string { return s.now().Format(time.RFC3339) }

// removeBoardFiles best-effort deletes attachment files from the upload dir.
func (s *Service) removeBoardFiles(attIDs []string) {
	for _, id := range attIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		_ = os.Remove(filepath.Join(s.uploadDir, id))
	}
}

/* ---- board view ---------------------------------------------------------- */

// BoardUser is one roster entry for the member picker (cross-division).
type BoardUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Division string `json:"division"`
}

// BoardMe describes the caller to the frontend.
type BoardMe struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Admin    bool   `json:"admin"`
	Division string `json:"division"`
}

// BoardView is the full GET /api/board payload. Lists carry the mixed card view
// (stored free cards + the caller's read-model task cards).
type BoardView struct {
	Lists       []boardListView     `json:"lists"`
	Labels      []domain.BoardLabel `json:"labels"`
	Users       []BoardUser         `json:"users"`
	Departments []domain.Department `json:"departments"`
	Me          BoardMe             `json:"me"`
}

// boardListView mirrors domain.BoardList in the GET response, but its cards are
// the mixed free-card + task-card view (boardCardView).
type boardListView struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	System    bool            `json:"system,omitempty"`
	CreatedBy string          `json:"createdBy"`
	Cards     []boardCardView `json:"cards"`
}

// boardCardView is one entry in a list's cards array in GET /api/board. It is
// EITHER a stored free card (type:"card") OR a read-model formal-task card
// (type:"task") — exactly one of the two is set. Custom marshaling emits the
// documented shape for each kind (task cards omit desc/createdAt/updatedAt).
type boardCardView struct {
	free *domain.BoardCard
	task *taskCardView
}

// MarshalJSON renders a free card as its BoardCard fields plus "type":"card",
// and a task card as its explicit task shape.
func (v boardCardView) MarshalJSON() ([]byte, error) {
	if v.task != nil {
		return json.Marshal(v.task)
	}
	type alias domain.BoardCard // strip BoardCard's own (none) marshaler; keep tags
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "card", alias: alias(*v.free)})
}

// taskCardView is the read-model card projected from a formal domain.Task. It is
// per-caller and NOT persisted into the board store.
type taskCardView struct {
	Type        string           `json:"type"` // always "task"
	ID          string           `json:"id"`   // task-<projectId>-<taskId>
	ListID      string           `json:"listId"`
	Title       string           `json:"title"`
	Members     []string         `json:"members"`
	Labels      []string         `json:"labels"`
	Checklists  []any                 `json:"checklists"`
	Attachments []any                 `json:"attachments"`
	Comments    []domain.BoardComment `json:"comments"`
	Due         string           `json:"due"`
	DueDone     bool             `json:"dueDone"`
	Cover       string           `json:"cover"`
	Division    string           `json:"division"`
	CreatedBy   string           `json:"createdBy"`
	Project     boardCardProject `json:"project"`
	Task        boardCardTask    `json:"task"`
}

// boardCardProject is the parent-project summary embedded in a task card.
type boardCardProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	GP   string `json:"gp"`
}

// boardCardTask is the formal-task metadata embedded in a task card; it carries
// everything the board UI needs to drive the task-action proxy endpoints.
type boardCardTask struct {
	ProjectID  string            `json:"projectId"`
	TaskID     string            `json:"taskId"`
	Category   string            `json:"category"`
	Group      string            `json:"group"`
	PIC        string            `json:"pic"`
	Output     domain.Division   `json:"output"`
	Status     domain.TaskStatus `json:"status"`
	Weighted   bool              `json:"weighted"`
	HasDoc     bool              `json:"hasDoc"`
	ApprovedBy string            `json:"approvedBy"`
	ApprovedAt string            `json:"approvedAt"`
	RevisiNote string            `json:"revisiNote"`
	UpdatedAt  string            `json:"updatedAt"`
}

// buildTaskCard projects one of the caller's assigned tasks into a task card.
func buildTaskCard(at AssignedTask) boardCardView {
	t := at.Task
	// Project the task's attachments into the SAME JSON the frontend expects for a
	// BoardAttachment ({id,name,size,mime,by,at}); keep [] (never null) when none.
	atts := make([]any, 0, len(t.Attachments))
	for _, a := range t.Attachments {
		atts = append(atts, a)
	}
	return boardCardView{task: &taskCardView{
		Type:        "task",
		ID:          "task-" + at.ProjectID + "-" + t.ID,
		ListID:      domain.SysListForStatus(t.Status),
		Title:       t.Name,
		Members:     []string{t.PIC},
		Labels:      []string{},
		Checklists:  []any{},
		Attachments: atts,
		Comments:    append([]domain.BoardComment{}, t.Comments...),
		Due:         "",
		DueDone:     false,
		Cover:       "",
		Division:    "perencanaan",
		CreatedBy:   t.PIC,
		Project:     boardCardProject{ID: at.ProjectID, Name: at.ProjectName, GP: at.GP},
		Task: boardCardTask{
			ProjectID:  at.ProjectID,
			TaskID:     t.ID,
			Category:   t.Category,
			Group:      t.Group,
			PIC:        t.PIC,
			Output:     t.Output,
			Status:     t.Status,
			Weighted:   t.Weighted,
			HasDoc:     t.Doc != nil,
			ApprovedBy: t.ApprovedBy,
			ApprovedAt: t.ApprovedAt,
			RevisiNote: t.RevisiNote,
			UpdatedAt:  t.UpdatedAt,
		},
	}}
}

// Board returns the whole board plus the CROSS-DIVISION roster, the department
// catalogue and caller info. The perencanaan SSO roster sync still runs
// best-effort first (keeps the PIC store fresh, same as Staff); the board
// roster itself comes from the TTL-cached cross-division fetch.
//
// The board is a STATUS board: the 4 fixed system lists carry their stored FREE
// cards, and the CALLER's own formal tasks are injected as read-model task cards
// (single source of truth = projects; never copied into the board store),
// appended to the status list matching each task's status.
func (s *Service) Board(actor domain.User, token string) BoardView {
	s.syncFromAuth(token)
	users, depts := s.crossRoster(token)
	division := strings.ToLower(strings.TrimSpace(actor.Division))
	if division == "" {
		division = "perencanaan"
	}

	// System lists with their stored FREE cards.
	lists := s.repo.Board()
	views := make([]boardListView, 0, len(lists))
	idx := map[string]int{} // list id -> index in views (for task-card injection)
	for _, l := range lists {
		lv := boardListView{ID: l.ID, Title: l.Title, System: l.System, CreatedBy: l.CreatedBy, Cards: make([]boardCardView, 0, len(l.Cards))}
		for i := range l.Cards {
			c := l.Cards[i]
			lv.Cards = append(lv.Cards, boardCardView{free: &c})
		}
		idx[l.ID] = len(views)
		views = append(views, lv)
	}

	// Inject the caller's formal tasks as task cards, appended AFTER the free
	// cards of the status list matching each task's status.
	for _, at := range s.TasksForPIC(actor.Username) {
		pos, ok := idx[domain.SysListForStatus(at.Status)]
		if !ok {
			pos, ok = idx[domain.SysListTodo] // status maps to a missing list -> To Do
		}
		if ok {
			views[pos].Cards = append(views[pos].Cards, buildTaskCard(at))
		}
	}

	return BoardView{
		Lists:       views,
		Labels:      s.repo.BoardLabels(),
		Users:       users,
		Departments: depts,
		Me:          BoardMe{Username: actor.Username, Role: actor.Role, Admin: canManage(actor.Role), Division: division},
	}
}

/* ---- lists ---------------------------------------------------------------- */

// BoardAddList is DISABLED: the board is a fixed status board (the 4 system
// columns), so no new columns may be created.
func (s *Service) BoardAddList(_ domain.User, _ string) (domain.BoardList, error) {
	return domain.BoardList{}, fmt.Errorf("%w: kolom papan bersifat tetap (To Do/Progress/Review/Selesai)", ErrForbidden)
}

// BoardUpdateList renames and/or repositions a list (admin or list creator).
func (s *Service) BoardUpdateList(actor domain.User, listID string, title *string, index *int) (domain.BoardList, error) {
	l, ok := s.repo.BoardListByID(listID)
	if !ok {
		return domain.BoardList{}, ErrNotFound
	}
	if l.System {
		return domain.BoardList{}, fmt.Errorf("%w: kolom papan bersifat tetap (To Do/Progress/Review/Selesai)", ErrForbidden)
	}
	if !canManage(actor.Role) && l.CreatedBy != actor.Username {
		return domain.BoardList{}, ErrForbidden
	}
	if title == nil && index == nil {
		return domain.BoardList{}, fmt.Errorf("%w: tidak ada perubahan (title/index)", ErrValidation)
	}
	if title != nil {
		t := strings.TrimSpace(*title)
		if t == "" {
			return domain.BoardList{}, fmt.Errorf("%w: judul list wajib diisi", ErrValidation)
		}
		title = &t
	}
	out, ok := s.repo.UpdateBoardList(listID, title, index)
	if !ok {
		return domain.BoardList{}, ErrNotFound
	}
	return out, nil
}

// BoardDeleteList removes a list, its cards and their attachment files
// (admin only).
func (s *Service) BoardDeleteList(actor domain.User, listID string) error {
	if !canManage(actor.Role) {
		return ErrForbidden
	}
	if l, ok := s.repo.BoardListByID(listID); ok && l.System {
		return fmt.Errorf("%w: kolom papan bersifat tetap (To Do/Progress/Review/Selesai)", ErrForbidden)
	}
	atts, ok := s.repo.DeleteBoardList(listID)
	if !ok {
		return ErrNotFound
	}
	s.removeBoardFiles(atts)
	return nil
}

/* ---- cards ---------------------------------------------------------------- */

// BoardAddCard creates a card in a list (any authenticated user); the creator
// automatically becomes a member.
func (s *Service) BoardAddCard(actor domain.User, listID, title string) (domain.BoardCard, error) {
	title = strings.TrimSpace(title)
	if title == "" || strings.TrimSpace(listID) == "" {
		return domain.BoardCard{}, fmt.Errorf("%w: listId dan title wajib diisi", ErrValidation)
	}
	if !domain.IsSystemList(strings.TrimSpace(listID)) {
		return domain.BoardCard{}, fmt.Errorf("%w: kartu hanya bisa dibuat di kolom status (To Do/Progress/Review/Selesai)", ErrValidation)
	}
	now := s.nowRFC3339()
	card, ok := s.repo.AddBoardCard(listID, domain.BoardCard{
		Title:     title,
		Members:   []string{actor.Username},
		CreatedBy: actor.Username,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if !ok {
		return domain.BoardCard{}, ErrNotFound
	}
	return card, nil
}

// BoardCardByID returns one card (any authenticated user may view).
func (s *Service) BoardCardByID(cardID string) (domain.BoardCard, error) {
	c, ok := s.repo.BoardCard(cardID)
	if !ok {
		return domain.BoardCard{}, ErrNotFound
	}
	return c, nil
}

// BoardCanEditCard reports (as an error) whether actor is a contributor on the
// card — used by the upload handler BEFORE it consumes a potentially huge body.
func (s *Service) BoardCanEditCard(actor domain.User, cardID string) error {
	c, ok := s.repo.BoardCard(cardID)
	if !ok {
		return ErrNotFound
	}
	if !boardContributor(actor, c) {
		return ErrForbidden
	}
	return nil
}

// BoardCardPatch is the PATCH /api/board/cards/{cardId} body. nil = unchanged;
// Due "" clears the due date; Cover "" clears the cover; ListID/Index move;
// Division "" clears the card's division tag.
type BoardCardPatch struct {
	Title    *string `json:"title"`
	Desc     *string `json:"desc"`
	Due      *string `json:"due"`
	DueDone  *bool   `json:"dueDone"`
	ListID   *string `json:"listId"`
	Index    *int    `json:"index"`
	Cover    *string `json:"cover"`
	Division *string `json:"division"`
}

// BoardUpdateCard edits card fields and/or moves it (contributor only).
func (s *Service) BoardUpdateCard(actor domain.User, cardID string, p BoardCardPatch) (domain.BoardCard, error) {
	now := s.nowRFC3339()
	// Resolve the valid division set BEFORE entering the card mutation: the
	// mutation callback runs under the repository lock, and knownDeptCodes reads
	// repo.Departments() (would self-deadlock inside).
	var knownDivs map[string]bool
	if p.Division != nil {
		knownDivs = s.knownDeptCodes()
	}
	card, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		if p.Title != nil {
			t := strings.TrimSpace(*p.Title)
			if t == "" {
				return fmt.Errorf("%w: judul kartu wajib diisi", ErrValidation)
			}
			c.Title = t
		}
		if p.Desc != nil {
			c.Desc = *p.Desc
		}
		if p.Due != nil {
			due, err := rfc3339OrEmpty(*p.Due)
			if err != nil {
				return err
			}
			c.Due = due
		}
		if p.DueDone != nil {
			c.DueDone = *p.DueDone
		}
		if p.Cover != nil {
			cover := strings.TrimSpace(*p.Cover)
			if cover != "" {
				ok := false
				for _, a := range c.Attachments {
					if a.ID == cover {
						ok = true
						break
					}
				}
				if !ok {
					return fmt.Errorf("%w: lampiran cover tidak ditemukan di kartu ini", ErrValidation)
				}
			}
			c.Cover = cover
		}
		if p.Division != nil {
			div := strings.ToLower(strings.TrimSpace(*p.Division))
			if div != "" && !knownDivs[div] {
				return fmt.Errorf("%w: divisi %q tidak dikenal", ErrValidation, div)
			}
			c.Division = div
		}
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardCard{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardCard{}, err
	}
	// Move / reorder after the field edits (permission was already enforced).
	if p.ListID != nil || p.Index != nil {
		target := card.ListID
		if p.ListID != nil && strings.TrimSpace(*p.ListID) != "" {
			target = strings.TrimSpace(*p.ListID)
		}
		if !domain.IsSystemList(target) {
			return domain.BoardCard{}, fmt.Errorf("%w: kolom tujuan harus kolom status (To Do/Progress/Review/Selesai)", ErrValidation)
		}
		index := int(^uint(0) >> 1) // default: append at the end (clamped)
		if p.Index != nil {
			index = *p.Index
		}
		moved, ok := s.repo.MoveBoardCard(cardID, target, index, now)
		if !ok {
			return domain.BoardCard{}, ErrNotFound
		}
		card = moved
	}
	return card, nil
}

// BoardDeleteCard removes a card + its attachment files (contributor: admin,
// creator, or card member).
func (s *Service) BoardDeleteCard(actor domain.User, cardID string) error {
	c, ok := s.repo.BoardCard(cardID)
	if !ok {
		return ErrNotFound
	}
	if !boardContributor(actor, c) {
		return ErrForbidden
	}
	atts, ok := s.repo.DeleteBoardCard(cardID)
	if !ok {
		return ErrNotFound
	}
	s.removeBoardFiles(atts)
	return nil
}

/* ---- members --------------------------------------------------------------- */

// BoardAddMember adds a roster user to a card (contributor only; idempotent).
// The username is validated against the CROSS-DIVISION roster (any employee
// from any department), not just perencanaan's PIC accounts.
func (s *Service) BoardAddMember(actor domain.User, token, cardID, username string) (domain.BoardCard, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return domain.BoardCard{}, fmt.Errorf("%w: username wajib diisi", ErrValidation)
	}
	if !s.boardRosterHas(token, username) {
		return domain.BoardCard{}, fmt.Errorf("%w: user %q tidak ditemukan di roster", ErrValidation, username)
	}
	now := s.nowRFC3339()
	card, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for _, m := range c.Members {
			if m == username {
				return nil // already a member — idempotent
			}
		}
		c.Members = append(c.Members, username)
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardCard{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardCard{}, err
	}
	return card, nil
}

// BoardRemoveMember removes a member from a card (contributor only).
func (s *Service) BoardRemoveMember(actor domain.User, cardID, username string) (domain.BoardCard, error) {
	now := s.nowRFC3339()
	card, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		kept := c.Members[:0]
		for _, m := range c.Members {
			if m != username {
				kept = append(kept, m)
			}
		}
		c.Members = kept
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardCard{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardCard{}, err
	}
	return card, nil
}

/* ---- labels ---------------------------------------------------------------- */

// BoardAddLabel creates a board-wide label definition (any authenticated user).
func (s *Service) BoardAddLabel(name, color string) (domain.BoardLabel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.BoardLabel{}, fmt.Errorf("%w: nama label wajib diisi", ErrValidation)
	}
	return s.repo.AddBoardLabel(name, strings.TrimSpace(color)), nil
}

// BoardUpdateLabel edits a label definition (admin only).
func (s *Service) BoardUpdateLabel(actor domain.User, labelID string, name, color *string) (domain.BoardLabel, error) {
	if !canManage(actor.Role) {
		return domain.BoardLabel{}, ErrForbidden
	}
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return domain.BoardLabel{}, fmt.Errorf("%w: nama label wajib diisi", ErrValidation)
		}
		name = &n
	}
	lb, ok := s.repo.UpdateBoardLabel(labelID, name, color)
	if !ok {
		return domain.BoardLabel{}, ErrNotFound
	}
	return lb, nil
}

// BoardDeleteLabel removes a label definition and strips it from every card
// (admin only).
func (s *Service) BoardDeleteLabel(actor domain.User, labelID string) error {
	if !canManage(actor.Role) {
		return ErrForbidden
	}
	if !s.repo.DeleteBoardLabel(labelID) {
		return ErrNotFound
	}
	return nil
}

// BoardCardAddLabel attaches an existing label to a card (contributor only;
// idempotent).
func (s *Service) BoardCardAddLabel(actor domain.User, cardID, labelID string) (domain.BoardCard, error) {
	exists := false
	for _, lb := range s.repo.BoardLabels() {
		if lb.ID == labelID {
			exists = true
			break
		}
	}
	if !exists {
		return domain.BoardCard{}, ErrNotFound
	}
	now := s.nowRFC3339()
	card, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for _, id := range c.Labels {
			if id == labelID {
				return nil // idempotent
			}
		}
		c.Labels = append(c.Labels, labelID)
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardCard{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardCard{}, err
	}
	return card, nil
}

// BoardCardRemoveLabel detaches a label from a card (contributor only).
func (s *Service) BoardCardRemoveLabel(actor domain.User, cardID, labelID string) (domain.BoardCard, error) {
	now := s.nowRFC3339()
	card, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		kept := c.Labels[:0]
		for _, id := range c.Labels {
			if id != labelID {
				kept = append(kept, id)
			}
		}
		c.Labels = kept
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardCard{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardCard{}, err
	}
	return card, nil
}

/* ---- checklists ------------------------------------------------------------ */

// BoardAddChecklist adds a checklist to a card (contributor only).
func (s *Service) BoardAddChecklist(actor domain.User, cardID, title string) (domain.BoardChecklist, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return domain.BoardChecklist{}, fmt.Errorf("%w: judul checklist wajib diisi", ErrValidation)
	}
	now := s.nowRFC3339()
	var out domain.BoardChecklist
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, newID func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		out = domain.BoardChecklist{ID: newID("cl"), Title: title, Items: []domain.BoardChecklistItem{}}
		c.Checklists = append(c.Checklists, out)
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardChecklist{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardChecklist{}, err
	}
	return out, nil
}

// BoardUpdateChecklist renames a checklist (contributor only).
func (s *Service) BoardUpdateChecklist(actor domain.User, cardID, clID string, title *string) (domain.BoardChecklist, error) {
	now := s.nowRFC3339()
	var out domain.BoardChecklist
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for i := range c.Checklists {
			if c.Checklists[i].ID != clID {
				continue
			}
			if title != nil {
				t := strings.TrimSpace(*title)
				if t == "" {
					return fmt.Errorf("%w: judul checklist wajib diisi", ErrValidation)
				}
				c.Checklists[i].Title = t
			}
			out = c.Checklists[i]
			c.UpdatedAt = now
			return nil
		}
		return ErrNotFound
	})
	if !found {
		return domain.BoardChecklist{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardChecklist{}, err
	}
	return out, nil
}

// BoardDeleteChecklist removes a checklist (contributor only).
func (s *Service) BoardDeleteChecklist(actor domain.User, cardID, clID string) error {
	now := s.nowRFC3339()
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for i := range c.Checklists {
			if c.Checklists[i].ID == clID {
				c.Checklists = append(c.Checklists[:i], c.Checklists[i+1:]...)
				c.UpdatedAt = now
				return nil
			}
		}
		return ErrNotFound
	})
	if !found {
		return ErrNotFound
	}
	return err
}

// BoardAddChecklistItem appends an item to a checklist (contributor only).
func (s *Service) BoardAddChecklistItem(actor domain.User, cardID, clID, text, due string) (domain.BoardChecklistItem, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.BoardChecklistItem{}, fmt.Errorf("%w: teks item wajib diisi", ErrValidation)
	}
	due, err := rfc3339OrEmpty(due)
	if err != nil {
		return domain.BoardChecklistItem{}, err
	}
	now := s.nowRFC3339()
	var out domain.BoardChecklistItem
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, newID func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for i := range c.Checklists {
			if c.Checklists[i].ID == clID {
				out = domain.BoardChecklistItem{ID: newID("it"), Text: text, Due: due}
				c.Checklists[i].Items = append(c.Checklists[i].Items, out)
				c.UpdatedAt = now
				return nil
			}
		}
		return ErrNotFound
	})
	if !found {
		return domain.BoardChecklistItem{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardChecklistItem{}, err
	}
	return out, nil
}

// BoardUpdateChecklistItem edits an item (contributor only). Flipping done
// false→true stamps DoneAt=now; true→false clears it.
func (s *Service) BoardUpdateChecklistItem(actor domain.User, cardID, clID, itemID string, text *string, done *bool, due *string) (domain.BoardChecklistItem, error) {
	now := s.nowRFC3339()
	var out domain.BoardChecklistItem
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for i := range c.Checklists {
			if c.Checklists[i].ID != clID {
				continue
			}
			for j := range c.Checklists[i].Items {
				it := &c.Checklists[i].Items[j]
				if it.ID != itemID {
					continue
				}
				if text != nil {
					t := strings.TrimSpace(*text)
					if t == "" {
						return fmt.Errorf("%w: teks item wajib diisi", ErrValidation)
					}
					it.Text = t
				}
				if due != nil {
					d, err := rfc3339OrEmpty(*due)
					if err != nil {
						return err
					}
					it.Due = d
				}
				if done != nil && *done != it.Done {
					it.Done = *done
					if *done {
						it.DoneAt = now
					} else {
						it.DoneAt = ""
					}
				}
				out = *it
				c.UpdatedAt = now
				return nil
			}
			return ErrNotFound
		}
		return ErrNotFound
	})
	if !found {
		return domain.BoardChecklistItem{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardChecklistItem{}, err
	}
	return out, nil
}

// BoardDeleteChecklistItem removes an item (contributor only).
func (s *Service) BoardDeleteChecklistItem(actor domain.User, cardID, clID, itemID string) error {
	now := s.nowRFC3339()
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for i := range c.Checklists {
			if c.Checklists[i].ID != clID {
				continue
			}
			for j := range c.Checklists[i].Items {
				if c.Checklists[i].Items[j].ID == itemID {
					c.Checklists[i].Items = append(c.Checklists[i].Items[:j], c.Checklists[i].Items[j+1:]...)
					c.UpdatedAt = now
					return nil
				}
			}
			return ErrNotFound
		}
		return ErrNotFound
	})
	if !found {
		return ErrNotFound
	}
	return err
}

/* ---- attachments ------------------------------------------------------------ */

// BoardAddAttachment registers an already-streamed upload: tmpPath is a fully
// written temp file inside the upload dir. On success the file is renamed to
// its attachment ID and the metadata is stored; on any error the CALLER removes
// the temp file. Contributor only.
func (s *Service) BoardAddAttachment(actor domain.User, cardID, filename, partMime, tmpPath string, size int64) (domain.BoardAttachment, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "lampiran"
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
	if mimeType == "" {
		mimeType = strings.TrimSpace(partMime)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	now := s.nowRFC3339()
	var att domain.BoardAttachment
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, newID func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		id := newID("att")
		// Rename INSIDE the mutation so metadata is only committed when the
		// file made it to its final name (same dir → same volume → cheap).
		if err := os.Rename(tmpPath, filepath.Join(s.uploadDir, id)); err != nil {
			return fmt.Errorf("menyimpan file lampiran: %w", err)
		}
		att = domain.BoardAttachment{ID: id, Name: filename, Size: size, Mime: mimeType, By: actor.Username, At: now}
		c.Attachments = append(c.Attachments, att)
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardAttachment{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardAttachment{}, err
	}
	return att, nil
}

// BoardAttachment resolves attachment metadata + its file path (any
// authenticated user may download).
func (s *Service) BoardAttachment(attID string) (domain.BoardAttachment, string, error) {
	for _, l := range s.repo.Board() {
		for _, c := range l.Cards {
			for _, a := range c.Attachments {
				if a.ID == attID {
					return a, filepath.Join(s.uploadDir, a.ID), nil
				}
			}
		}
	}
	return domain.BoardAttachment{}, "", ErrNotFound
}

// BoardDeleteAttachment removes an attachment (contributor only), clears the
// cover if it pointed there, and deletes the file from disk (best-effort).
func (s *Service) BoardDeleteAttachment(actor domain.User, cardID, attID string) error {
	now := s.nowRFC3339()
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		if !boardContributor(actor, *c) {
			return ErrForbidden
		}
		for i := range c.Attachments {
			if c.Attachments[i].ID == attID {
				c.Attachments = append(c.Attachments[:i], c.Attachments[i+1:]...)
				if c.Cover == attID {
					c.Cover = ""
				}
				c.UpdatedAt = now
				return nil
			}
		}
		return ErrNotFound
	})
	if !found {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	s.removeBoardFiles([]string{attID})
	return nil
}

/* ---- comments ---------------------------------------------------------------- */

// BoardAddComment adds a comment to a card (ANY authenticated user).
func (s *Service) BoardAddComment(actor domain.User, cardID, text string) (domain.BoardComment, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.BoardComment{}, fmt.Errorf("%w: teks komentar wajib diisi", ErrValidation)
	}
	now := s.nowRFC3339()
	var out domain.BoardComment
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, newID func(string) string) error {
		out = domain.BoardComment{ID: newID("cm"), Author: actor.Username, Text: text, At: now}
		c.Comments = append(c.Comments, out)
		c.UpdatedAt = now
		return nil
	})
	if !found {
		return domain.BoardComment{}, ErrNotFound
	}
	if err != nil {
		return domain.BoardComment{}, err
	}
	return out, nil
}

// BoardDeleteComment removes a comment (its author or an admin).
func (s *Service) BoardDeleteComment(actor domain.User, cardID, commentID string) error {
	now := s.nowRFC3339()
	_, found, err := s.repo.MutateBoardCard(cardID, func(c *domain.BoardCard, _ func(string) string) error {
		for i := range c.Comments {
			if c.Comments[i].ID != commentID {
				continue
			}
			if c.Comments[i].Author != actor.Username && !canManage(actor.Role) {
				return ErrForbidden
			}
			c.Comments = append(c.Comments[:i], c.Comments[i+1:]...)
			c.UpdatedAt = now
			return nil
		}
		return ErrNotFound
	})
	if !found {
		return ErrNotFound
	}
	return err
}
