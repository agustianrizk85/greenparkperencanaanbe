package repository

import (
	"testing"

	"greenpark/perencanaan/internal/domain"
)

// TestEnsureBoardSystemListsIdempotent verifies the fixed status columns are
// created exactly once, legacy non-system lists fold into To Do, and a repeat
// call is a no-op.
func TestEnsureBoardSystemListsIdempotent(t *testing.T) {
	m := NewMemory()

	// Seed two legacy NON-system lists, each with a card.
	l1 := m.AddBoardList("Backlog", "u1")
	if _, ok := m.AddBoardCard(l1.ID, domain.BoardCard{Title: "A"}); !ok {
		t.Fatalf("seed card A failed")
	}
	l2 := m.AddBoardList("Doing", "u2")
	if _, ok := m.AddBoardCard(l2.ID, domain.BoardCard{Title: "B"}); !ok {
		t.Fatalf("seed card B failed")
	}

	assertSystemBoard := func(step string) {
		t.Helper()
		lists := m.Board()
		if len(lists) != len(domain.SystemBoardLists) {
			t.Fatalf("%s: got %d lists, want %d", step, len(lists), len(domain.SystemBoardLists))
		}
		for i, sys := range domain.SystemBoardLists {
			if lists[i].ID != sys.ID || lists[i].Title != sys.Title || !lists[i].System {
				t.Fatalf("%s: list[%d] = {%s,%q,system=%v}, want {%s,%q,system=true}",
					step, i, lists[i].ID, lists[i].Title, lists[i].System, sys.ID, sys.Title)
			}
		}
	}

	m.EnsureBoardSystemLists()
	assertSystemBoard("after first ensure")

	// Both legacy cards folded into To Do, re-homed to sys-todo.
	todo, ok := m.BoardListByID(domain.SysListTodo)
	if !ok {
		t.Fatalf("sys-todo missing")
	}
	if len(todo.Cards) != 2 {
		t.Fatalf("To Do has %d cards, want 2 folded", len(todo.Cards))
	}
	for _, c := range todo.Cards {
		if c.ListID != domain.SysListTodo {
			t.Fatalf("folded card %s has listId %q, want %q", c.ID, c.ListID, domain.SysListTodo)
		}
	}

	// Idempotent: a second run keeps exactly the 4 lists and the folded cards.
	m.EnsureBoardSystemLists()
	assertSystemBoard("after second ensure")
	if todo2, _ := m.BoardListByID(domain.SysListTodo); len(todo2.Cards) != 2 {
		t.Fatalf("after second ensure: To Do has %d cards, want 2", len(todo2.Cards))
	}
}
