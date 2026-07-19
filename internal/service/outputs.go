package service

import "greenpark/perencanaan/internal/domain"

// OutputItem is one finished-or-pending deliverable routed to a division.
type OutputItem struct {
	ProjectID   string            `json:"projectId"`
	ProjectName string            `json:"projectName"`
	GP          string            `json:"gp"`
	Deliverable string            `json:"deliverable"`
	PIC         string            `json:"pic"`
	Status      domain.TaskStatus `json:"status"`
	Ready       bool              `json:"ready"`
}

// DivisionOutputs bundles every deliverable feeding one division.
type DivisionOutputs struct {
	Division domain.Division `json:"division"`
	Label    string          `json:"label"`
	Ready    int             `json:"ready"`
	Total    int             `json:"total"`
	Items    []OutputItem    `json:"items"`
}

// wdConsumerDept / wdContractorDept are the department codes the working-drawing
// flow feeds into when those departments exist in the central catalogue.
const (
	wdConsumerDept   = "cso"    // gambar kerja konsumen
	wdContractorDept = "teknik" // gambar kerja kontraktor
)

// OutputsByDivision routes every deliverable to the DEPARTMENT that consumes it
// (the "output" section of the business process), over the dynamic department
// catalogue synced from auth SSO. A task feeds the department named in its
// Output; the per-consumer working-drawing flow feeds the consumer + contractor
// departments when present.
func (s *Service) OutputsByDivision() []DivisionOutputs {
	projects := s.repo.Projects()
	depts := s.repo.Departments()

	known := make(map[domain.Division]bool, len(depts))
	for _, d := range depts {
		known[domain.Division(d.Code)] = true
	}

	buckets := map[domain.Division][]OutputItem{}
	add := func(div domain.Division, it OutputItem) {
		if known[div] { // ignore outputs to unknown/removed departments
			buckets[div] = append(buckets[div], it)
		}
	}

	for _, p := range projects {
		for _, t := range p.Tasks {
			if t.Output == domain.DivNone {
				continue
			}
			add(t.Output, OutputItem{
				ProjectID: p.ID, ProjectName: p.Name, GP: p.GP,
				Deliverable: t.Name, PIC: t.PIC, Status: t.Status,
				Ready: t.Status == domain.StatusDone,
			})
		}
	}

	// Fold the per-consumer working-drawing flow into the consumer + contractor
	// departments (only if those exist in the catalogue).
	for _, d := range s.repo.WorkDrawings() {
		pname := projectName(projects, d.ProjectID)
		add(domain.Division(wdConsumerDept), OutputItem{
			ProjectID: d.ProjectID, ProjectName: pname, GP: gpFor(projects, d.ProjectID),
			Deliverable: "Gambar kerja konsumen — " + d.Konsumen + " (" + d.Unit + ")",
			PIC:         d.PIC, Status: statusForWD(d, false), Ready: d.KonsumenDone != "",
		})
		add(domain.Division(wdContractorDept), OutputItem{
			ProjectID: d.ProjectID, ProjectName: pname, GP: gpFor(projects, d.ProjectID),
			Deliverable: "Gambar kerja kontraktor — " + d.Konsumen + " (" + d.Unit + ")",
			PIC:         d.PIC, Status: statusForWD(d, true), Ready: d.KontraktorDone != "",
		})
	}

	out := make([]DivisionOutputs, 0, len(depts))
	for _, d := range depts {
		div := domain.Division(d.Code)
		items := buckets[div]
		if items == nil {
			items = []OutputItem{} // serialise empty as [] (not null) for the frontend
		}
		ready := 0
		for _, it := range items {
			if it.Ready {
				ready++
			}
		}
		out = append(out, DivisionOutputs{
			Division: div, Label: d.Name, Items: items, Ready: ready, Total: len(items),
		})
	}
	return out
}

// statusForWD maps a work-drawing flow to a coarse task status for display.
// contractor=true reports the contractor leg, otherwise the consumer leg.
func statusForWD(d domain.WorkDrawing, contractor bool) domain.TaskStatus {
	if contractor {
		switch {
		case d.KontraktorDone != "":
			return domain.StatusDone
		case d.Status == domain.WDKontraktor:
			return domain.StatusProgress
		default:
			return domain.StatusTodo
		}
	}
	switch {
	case d.KonsumenDone != "":
		return domain.StatusDone
	case d.Status == domain.WDKonsumen:
		return domain.StatusProgress
	default:
		return domain.StatusTodo
	}
}

func projectName(projects []domain.Project, id string) string {
	for _, p := range projects {
		if p.ID == id {
			return p.Name
		}
	}
	return id
}

func gpFor(projects []domain.Project, id string) string {
	for _, p := range projects {
		if p.ID == id {
			return p.GP
		}
	}
	return ""
}
