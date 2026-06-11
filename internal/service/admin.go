package service

import (
	"time"

	"greenpark/perencanaan/internal/domain"
)

// ResetProses clears only PROCESS data (task status + working-drawing flows),
// preserving all MASTER data including manually added projects. Only CEO /
// Kadep may reset. Backs the "Reset Proses" action.
func (s *Service) ResetProses(role string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	s.repo.ResetProses()
	return nil
}

// ResetMaster rebuilds the MASTER portfolio back to the seeded 32 projects
// (dropping any added project) and clears process data too. Only CEO / Kadep
// may reset. Backs the "Reset Master" action.
func (s *Service) ResetMaster(role string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	s.repo.ResetMaster()
	return nil
}

// SeedDemo replaces all dynamic data with a realistic sample set so the
// dashboard is populated for demos and first runs: varied task progress across
// the portfolio plus a handful of working-drawing flows spanning every SLA
// state (on-track, near-due, overdue, signed, done). Only CEO / Kadep may seed.
func (s *Service) SeedDemo(role string) error {
	if !canManage(role) {
		return ErrForbidden
	}
	s.seedDemo()
	return nil
}

// SeedDemoSystem runs the demo seed without a permission check. Intended for
// startup wiring (see cmd/server) so a freshly started server already shows
// data; it is not exposed over HTTP.
func (s *Service) SeedDemoSystem() { s.seedDemo() }

// statusCycle is the deterministic progression used to spread task statuses so
// the seeded portfolio shows a believable mix rather than all-or-nothing.
var statusCycle = []domain.TaskStatus{
	domain.StatusDone, domain.StatusDone, domain.StatusReview,
	domain.StatusProgress, domain.StatusProgress, domain.StatusTodo,
}

func (s *Service) seedDemo() {
	s.repo.ResetMaster()
	at := s.now().Format(time.RFC3339)

	// Spread task statuses deterministically. Lower-numbered (older) projects
	// are further along; later tasks in each tree lag behind earlier ones.
	for _, p := range s.repo.Projects() {
		for i, t := range p.Tasks {
			idx := (p.No + i) % len(statusCycle)
			// Newer projects (higher No) start further back in the cycle.
			if p.No%4 == 0 {
				idx = (idx + 3) % len(statusCycle)
			}
			status := statusCycle[idx]
			if status != domain.StatusTodo {
				s.repo.UpdateTaskStatus(p.ID, t.ID, status, at)
			}
		}
	}

	// A spread of consumer working-drawing flows across every SLA state.
	// dates are calendar-days before today; the service computes the working-day
	// SLA gates from them.
	s.seedDrawingOverdue("gp-001", "Bpk. Andi Pratama", "A-12", domain.PicAgus, 24)
	s.seedDrawingActive("gp-002", "Ibu Sari Wahyuni", "B-03", domain.PicAgus, 4)
	s.seedDrawingActive("gp-004", "Bpk. Joko Santoso", "C-07", domain.PicAgus, 16)
	s.seedDrawingKontraktor("gp-005", "Ibu Maya Lestari", "D-01", domain.PicAgus, 22, 3)
	s.seedDrawingKontraktorOverdue("gp-007", "Bpk. Rudi Hartono", "E-09", domain.PicAgus, 30, 9)
	s.seedDrawingDone("gp-009", "Ibu Dewi Anggraini", "F-04", domain.PicAgus, 40, 18)
}

// daysAgo returns the date n calendar days before "today" as YYYY-MM-DD.
func (s *Service) daysAgo(n int) string {
	return s.now().AddDate(0, 0, -n).Format(domain.DateLayout)
}

// seedDrawingActive creates a flow still in the consumer leg (info masuk only).
func (s *Service) seedDrawingActive(projectID, konsumen, unit, pic string, infoDaysAgo int) {
	_, _ = s.CreateWorkDrawing(CreateWorkDrawingInput{
		ProjectID: projectID, Konsumen: konsumen, Unit: unit, PIC: pic,
		InfoMasuk: s.daysAgo(infoDaysAgo),
	})
}

// seedDrawingOverdue is an active consumer leg whose 15-hk SLA has lapsed.
func (s *Service) seedDrawingOverdue(projectID, konsumen, unit, pic string, infoDaysAgo int) {
	s.seedDrawingActive(projectID, konsumen, unit, pic, infoDaysAgo)
}

// seedDrawingKontraktor creates a flow advanced past consumer TTD into the
// contractor leg (5-hk SLA running), signed ttdDaysAgo days ago.
func (s *Service) seedDrawingKontraktor(projectID, konsumen, unit, pic string, infoDaysAgo, ttdDaysAgo int) {
	v, err := s.CreateWorkDrawing(CreateWorkDrawingInput{
		ProjectID: projectID, Konsumen: konsumen, Unit: unit, PIC: pic,
		InfoMasuk: s.daysAgo(infoDaysAgo),
	})
	if err != nil {
		return
	}
	_, _ = s.AdvanceWorkDrawing(v.ID, AdvanceWorkDrawingInput{
		Action: "ttd-konsumen", Date: s.daysAgo(ttdDaysAgo),
	})
}

// seedDrawingKontraktorOverdue is a contractor leg whose 5-hk SLA has lapsed.
func (s *Service) seedDrawingKontraktorOverdue(projectID, konsumen, unit, pic string, infoDaysAgo, ttdDaysAgo int) {
	s.seedDrawingKontraktor(projectID, konsumen, unit, pic, infoDaysAgo, ttdDaysAgo)
}

// seedDrawingDone creates a fully delivered flow (contractor drawing complete).
func (s *Service) seedDrawingDone(projectID, konsumen, unit, pic string, infoDaysAgo, ttdDaysAgo int) {
	v, err := s.CreateWorkDrawing(CreateWorkDrawingInput{
		ProjectID: projectID, Konsumen: konsumen, Unit: unit, PIC: pic,
		InfoMasuk: s.daysAgo(infoDaysAgo),
	})
	if err != nil {
		return
	}
	if _, err = s.AdvanceWorkDrawing(v.ID, AdvanceWorkDrawingInput{
		Action: "ttd-konsumen", Date: s.daysAgo(ttdDaysAgo),
	}); err != nil {
		return
	}
	_, _ = s.AdvanceWorkDrawing(v.ID, AdvanceWorkDrawingInput{
		Action: "kontraktor-selesai", Date: s.daysAgo(ttdDaysAgo - 4),
	})
}
