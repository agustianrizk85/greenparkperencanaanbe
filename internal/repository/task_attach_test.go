package repository

import (
	"strings"
	"testing"

	"greenpark/perencanaan/internal/domain"
)

// TestNextBoardIDMonotonicUnique verifies the shared board-id counter mints
// prefixed, monotonically-unique ids (task attachments draw from it too, so they
// never collide with board-card attachment files in the same upload dir).
func TestNextBoardIDMonotonicUnique(t *testing.T) {
	m := NewMemory()
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		id := m.NextBoardID("att")
		if !strings.HasPrefix(id, "att-") {
			t.Fatalf("id %q missing att- prefix", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
	// The counter is shared with board ids: a board attachment id must not repeat
	// one already handed out for a task attachment.
	if id := m.NextBoardID("att"); seen[id] {
		t.Fatalf("shared counter reused id %q", id)
	}
}

// TestTaskAttachmentRoundTrip verifies TaskAttachment metadata is appended by
// MutateTask and survives a snapshot round-trip (bytes live on disk, not here).
func TestTaskAttachmentRoundTrip(t *testing.T) {
	m := NewMemory()
	projects := m.Projects()
	if len(projects) == 0 || len(projects[0].Tasks) == 0 {
		t.Skip("seed has no project/task to attach to")
	}
	pid, tid := projects[0].ID, projects[0].Tasks[0].ID

	att := domain.TaskAttachment{ID: m.NextBoardID("att"), Name: "denah.dwg", Size: 1234, Mime: "application/octet-stream", By: "agus", At: "2026-07-21T00:00:00Z"}
	if !m.MutateTask(pid, tid, func(tk *domain.Task) { tk.Attachments = append(tk.Attachments, att) }) {
		t.Fatalf("MutateTask returned not-found for %s/%s", pid, tid)
	}

	// Snapshot -> restore into a fresh store: the attachment metadata must persist.
	snap, err := m.SnapshotJSON()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	m2 := NewMemory()
	if err := m2.LoadJSON(snap); err != nil {
		t.Fatalf("load: %v", err)
	}
	p2, ok := m2.Project(pid)
	if !ok {
		t.Fatalf("project %s missing after restore", pid)
	}
	var got *domain.Task
	for i := range p2.Tasks {
		if p2.Tasks[i].ID == tid {
			got = &p2.Tasks[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("task %s missing after restore", tid)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].ID != att.ID || got.Attachments[0].Name != "denah.dwg" {
		t.Fatalf("attachment did not round-trip: %+v", got.Attachments)
	}
}
