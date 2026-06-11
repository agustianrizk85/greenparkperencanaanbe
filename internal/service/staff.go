package service

import (
	"math"

	"greenpark/perencanaan/internal/domain"
)

// StaffMember is one department account enriched with its current workload.
// CEO / Kadep are listed for the org view but carry no deliverable tasks; the
// three authors (Randi, Ananto, Agus) own the tasks and gambar-kerja flows.
type StaffMember struct {
	Username       string `json:"username"`
	Name           string `json:"name"`
	Role           string `json:"role"`
	RoleLabel      string `json:"roleLabel"`
	IsPIC          bool   `json:"isPic"`
	Total          int    `json:"total"`          // tasks assigned
	Done           int    `json:"done"`           // tasks completed
	InProgress     int    `json:"inProgress"`     // progress + review
	Progress       int    `json:"progress"`       // 0-100, weighted
	ActiveDrawings int    `json:"activeDrawings"` // open gambar-kerja flows
}

var roleLabels = map[string]string{
	domain.RoleCEO:     "Direktur Utama",
	domain.RoleKadep:   "Kepala Departemen",
	domain.RoleArsitek: "Arsitek",
	domain.RoleDrafter: "Drafter",
}

// Staff returns the department roster with per-author workload statistics,
// backing the "Tim / Staff" view.
func (s *Service) Staff() []StaffMember {
	// Aggregate task load per PIC across the portfolio.
	total := map[string]int{}
	done := map[string]int{}
	inprog := map[string]int{}
	weight := map[string]float64{}
	for _, p := range s.repo.Projects() {
		for _, t := range p.Tasks {
			total[t.PIC]++
			weight[t.PIC] += t.Status.Weight()
			switch t.Status {
			case domain.StatusDone:
				done[t.PIC]++
			case domain.StatusProgress, domain.StatusReview:
				inprog[t.PIC]++
			}
		}
	}

	// Count active (not-done) working-drawing flows per PIC.
	activeWD := map[string]int{}
	for _, d := range s.repo.WorkDrawings() {
		if d.Status != domain.WDDone {
			activeWD[d.PIC]++
		}
	}

	out := []StaffMember{}
	for _, u := range s.repo.Users() {
		isPIC := u.Role == domain.RoleArsitek || u.Role == domain.RoleDrafter
		m := StaffMember{
			Username:       u.Username,
			Name:           u.Name,
			Role:           u.Role,
			RoleLabel:      roleLabels[u.Role],
			IsPIC:          isPIC,
			Total:          total[u.Username],
			Done:           done[u.Username],
			InProgress:     inprog[u.Username],
			ActiveDrawings: activeWD[u.Username],
		}
		if m.Total > 0 {
			m.Progress = int(math.Round(weight[u.Username] / float64(m.Total) * 100))
		}
		out = append(out, m)
	}
	return out
}
