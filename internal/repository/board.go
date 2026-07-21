package repository

import (
	"fmt"

	"greenpark/perencanaan/internal/domain"
)

// This file implements the department Kanban board on *Memory. All methods are
// mutex-guarded; reads return deep copies with every nested slice non-nil so
// JSON responses never contain null where the contract promises [].

/* ---- clone helpers ------------------------------------------------------ */

// cloneBoardCard deep-copies a card, normalising nil slices to empty ones.
func cloneBoardCard(c domain.BoardCard) domain.BoardCard {
	out := c
	out.Members = append([]string{}, c.Members...)
	out.Labels = append([]string{}, c.Labels...)
	out.Checklists = make([]domain.BoardChecklist, len(c.Checklists))
	for i, cl := range c.Checklists {
		cl.Items = append([]domain.BoardChecklistItem{}, cl.Items...)
		out.Checklists[i] = cl
	}
	out.Attachments = append([]domain.BoardAttachment{}, c.Attachments...)
	out.Comments = append([]domain.BoardComment{}, c.Comments...)
	return out
}

// cloneBoardList deep-copies a list and its cards.
func cloneBoardList(l domain.BoardList) domain.BoardList {
	out := l
	out.Cards = make([]domain.BoardCard, len(l.Cards))
	for i, c := range l.Cards {
		out.Cards[i] = cloneBoardCard(c)
	}
	return out
}

// nextBoardID mints the next board-scoped ID (bl-3, cd-17, cl-4, it-9, att-2,
// lb-1, cm-5). Callers must hold m.mu.
func (m *Memory) nextBoardID(prefix string) string {
	m.boardSeq++
	return fmt.Sprintf("%s-%d", prefix, m.boardSeq)
}

// NextBoardID mints a fresh board-scoped ID from the SHARED counter (thread-safe
// wrapper over nextBoardID). Formal-task attachments live in the SAME upload dir
// as board-card attachments, so they draw their ids from this one sequence to
// guarantee filenames never collide. The bumped counter is persisted by the very
// next MutateTask save (Persistent), so a successful upload always round-trips.
func (m *Memory) NextBoardID(prefix string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nextBoardID(prefix)
}

// findBoardCard locates a card. Callers must hold m.mu. Returns the owning
// list, the card index within it, or ok=false.
func (m *Memory) findBoardCard(cardID string) (*domain.BoardList, int, bool) {
	for _, l := range m.boardLists {
		for i := range l.Cards {
			if l.Cards[i].ID == cardID {
				return l, i, true
			}
		}
	}
	return nil, 0, false
}

// findBoardList locates a list by ID. Callers must hold m.mu.
func (m *Memory) findBoardList(listID string) (*domain.BoardList, int, bool) {
	for i, l := range m.boardLists {
		if l.ID == listID {
			return l, i, true
		}
	}
	return nil, 0, false
}

/* ---- system status lists (fixed columns) --------------------------------- */

// EnsureBoardSystemLists makes the board a STATUS board: it guarantees the 4
// fixed system lists (To Do / Sedang Dikerjakan / Review / Selesai) exist with
// their literal ids, titles and order at the FRONT of the board, and folds every
// pre-existing NON-system list into To Do (one-time migration). Idempotent — safe
// to run on a fresh seed OR when restoring an old snapshot that predates the
// system lists. Callers need not hold m.mu.
func (m *Memory) EnsureBoardSystemLists() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Index the current lists by id.
	existing := map[string]*domain.BoardList{}
	for _, l := range m.boardLists {
		existing[l.ID] = l
	}

	// Ensure every system list exists and is normalised (title/order/flag fixed).
	for _, sys := range domain.SystemBoardLists {
		l, ok := existing[sys.ID]
		if !ok {
			l = &domain.BoardList{ID: sys.ID, Title: sys.Title, System: true, CreatedBy: "", Cards: []domain.BoardCard{}}
			existing[sys.ID] = l
		} else {
			l.Title = sys.Title
			l.System = true
			l.CreatedBy = ""
		}
	}

	// MIGRATION: fold every card from a NON-system list into To Do (append,
	// preserving order), then drop the now-empty non-system lists.
	todo := existing[domain.SysListTodo]
	for _, l := range m.boardLists {
		if domain.IsSystemList(l.ID) {
			continue
		}
		todo.Cards = append(todo.Cards, l.Cards...)
	}
	for i := range todo.Cards {
		todo.Cards[i].ListID = domain.SysListTodo // re-home folded cards
	}

	// Rebuild the board as exactly the 4 system lists in fixed order.
	rebuilt := make([]*domain.BoardList, 0, len(domain.SystemBoardLists))
	for _, sys := range domain.SystemBoardLists {
		rebuilt = append(rebuilt, existing[sys.ID])
	}
	m.boardLists = rebuilt
}

// ClearBoardCards empties every list of its cards while KEEPING the lists
// themselves (including the 4 fixed system columns). Used by the demo/"Isi
// Contoh" seed so repeated runs replace the sample cards instead of piling them
// up. Board labels are left intact. Callers must NOT hold m.mu.
func (m *Memory) ClearBoardCards() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.boardLists {
		l.Cards = []domain.BoardCard{}
	}
}

/* ---- reads --------------------------------------------------------------- */

// Board returns the whole board (lists in display order, cards in display
// order) as a deep copy.
func (m *Memory) Board() []domain.BoardList {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.BoardList, 0, len(m.boardLists))
	for _, l := range m.boardLists {
		out = append(out, cloneBoardList(*l))
	}
	return out
}

// BoardLabels returns the board-wide label definitions.
func (m *Memory) BoardLabels() []domain.BoardLabel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.BoardLabel, 0, len(m.boardLabels))
	out = append(out, m.boardLabels...)
	return out
}

// BoardCard returns a deep copy of one card.
func (m *Memory) BoardCard(cardID string) (domain.BoardCard, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, i, ok := m.findBoardCard(cardID)
	if !ok {
		return domain.BoardCard{}, false
	}
	return cloneBoardCard(l.Cards[i]), true
}

// BoardListByID returns a deep copy of one list.
func (m *Memory) BoardListByID(listID string) (domain.BoardList, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, _, ok := m.findBoardList(listID)
	if !ok {
		return domain.BoardList{}, false
	}
	return cloneBoardList(*l), true
}

/* ---- list mutations ------------------------------------------------------ */

// AddBoardList appends a new list (column) to the board.
func (m *Memory) AddBoardList(title, createdBy string) domain.BoardList {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := &domain.BoardList{
		ID:        m.nextBoardID("bl"),
		Title:     title,
		CreatedBy: createdBy,
		Cards:     []domain.BoardCard{},
	}
	m.boardLists = append(m.boardLists, l)
	return cloneBoardList(*l)
}

// UpdateBoardList renames and/or repositions a list. A nil field is left
// unchanged; index is clamped into the valid range.
func (m *Memory) UpdateBoardList(listID string, title *string, index *int) (domain.BoardList, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, pos, ok := m.findBoardList(listID)
	if !ok {
		return domain.BoardList{}, false
	}
	if title != nil {
		l.Title = *title
	}
	if index != nil {
		m.boardLists = append(m.boardLists[:pos], m.boardLists[pos+1:]...)
		idx := *index
		if idx < 0 {
			idx = 0
		}
		if idx > len(m.boardLists) {
			idx = len(m.boardLists)
		}
		m.boardLists = append(m.boardLists, nil)
		copy(m.boardLists[idx+1:], m.boardLists[idx:])
		m.boardLists[idx] = l
	}
	return cloneBoardList(*l), true
}

// DeleteBoardList removes a list and all its cards, returning the attachment
// IDs of every removed card so the caller can delete the files from disk.
func (m *Memory) DeleteBoardList(listID string) ([]string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, pos, ok := m.findBoardList(listID)
	if !ok {
		return nil, false
	}
	atts := []string{}
	for _, c := range l.Cards {
		for _, a := range c.Attachments {
			atts = append(atts, a.ID)
		}
	}
	m.boardLists = append(m.boardLists[:pos], m.boardLists[pos+1:]...)
	return atts, true
}

/* ---- card mutations ------------------------------------------------------ */

// AddBoardCard assigns an ID and appends the card to the given list. Returns
// ok=false when the list does not exist.
func (m *Memory) AddBoardCard(listID string, card domain.BoardCard) (domain.BoardCard, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, _, ok := m.findBoardList(listID)
	if !ok {
		return domain.BoardCard{}, false
	}
	card = cloneBoardCard(card) // normalise nil slices
	card.ID = m.nextBoardID("cd")
	card.ListID = l.ID
	l.Cards = append(l.Cards, card)
	return cloneBoardCard(card), true
}

// MutateBoardCard applies fn to a deep copy of the card under the lock; the
// copy replaces the stored card only when fn returns nil, so a failed edit
// leaves the board untouched. fn receives newID to mint board-scoped IDs
// (checklists, items, attachments, comments). Returns the updated copy, whether
// the card was found, and fn's error.
func (m *Memory) MutateBoardCard(cardID string, fn func(c *domain.BoardCard, newID func(prefix string) string) error) (domain.BoardCard, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, i, ok := m.findBoardCard(cardID)
	if !ok {
		return domain.BoardCard{}, false, nil
	}
	work := cloneBoardCard(l.Cards[i])
	if err := fn(&work, m.nextBoardID); err != nil {
		return domain.BoardCard{}, true, err
	}
	work.ID, work.ListID = cardID, l.ID // identity is immutable via mutation
	l.Cards[i] = work
	return cloneBoardCard(work), true, nil
}

// MoveBoardCard moves a card to position index of list toListID (which may be
// its current list, for a reorder). index is clamped; at stamps UpdatedAt.
func (m *Memory) MoveBoardCard(cardID, toListID string, index int, at string) (domain.BoardCard, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src, si, ok := m.findBoardCard(cardID)
	if !ok {
		return domain.BoardCard{}, false
	}
	dst, _, ok := m.findBoardList(toListID)
	if !ok {
		return domain.BoardCard{}, false
	}
	card := src.Cards[si]
	src.Cards = append(src.Cards[:si], src.Cards[si+1:]...)
	if index < 0 {
		index = 0
	}
	if index > len(dst.Cards) {
		index = len(dst.Cards)
	}
	card.ListID = dst.ID
	card.UpdatedAt = at
	dst.Cards = append(dst.Cards, domain.BoardCard{})
	copy(dst.Cards[index+1:], dst.Cards[index:])
	dst.Cards[index] = card
	return cloneBoardCard(card), true
}

// DeleteBoardCard removes a card, returning its attachment IDs so the caller
// can delete the files from disk.
func (m *Memory) DeleteBoardCard(cardID string) ([]string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, i, ok := m.findBoardCard(cardID)
	if !ok {
		return nil, false
	}
	atts := []string{}
	for _, a := range l.Cards[i].Attachments {
		atts = append(atts, a.ID)
	}
	l.Cards = append(l.Cards[:i], l.Cards[i+1:]...)
	return atts, true
}

/* ---- label mutations ------------------------------------------------------ */

// AddBoardLabel creates a new board-wide label definition.
func (m *Memory) AddBoardLabel(name, color string) domain.BoardLabel {
	m.mu.Lock()
	defer m.mu.Unlock()
	lb := domain.BoardLabel{ID: m.nextBoardID("lb"), Name: name, Color: color}
	m.boardLabels = append(m.boardLabels, lb)
	return lb
}

// UpdateBoardLabel edits a label definition (nil field = unchanged).
func (m *Memory) UpdateBoardLabel(labelID string, name, color *string) (domain.BoardLabel, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.boardLabels {
		if m.boardLabels[i].ID == labelID {
			if name != nil {
				m.boardLabels[i].Name = *name
			}
			if color != nil {
				m.boardLabels[i].Color = *color
			}
			return m.boardLabels[i], true
		}
	}
	return domain.BoardLabel{}, false
}

// DeleteBoardLabel removes a label definition and strips its ID from every card.
func (m *Memory) DeleteBoardLabel(labelID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for i := range m.boardLabels {
		if m.boardLabels[i].ID == labelID {
			m.boardLabels = append(m.boardLabels[:i], m.boardLabels[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return false
	}
	for _, l := range m.boardLists {
		for i := range l.Cards {
			kept := l.Cards[i].Labels[:0]
			for _, id := range l.Cards[i].Labels {
				if id != labelID {
					kept = append(kept, id)
				}
			}
			l.Cards[i].Labels = kept
		}
	}
	return true
}
