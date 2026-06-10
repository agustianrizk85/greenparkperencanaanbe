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

// divisionLabels gives the display name and output order of each division.
var divisionOrder = []struct {
	div   domain.Division
	label string
}{
	{domain.DivLegal, "Legal"},
	{domain.DivMarketing, "Marketing"},
	{domain.DivTeknik, "Teknik"},
	{domain.DivKonsumen, "Konsumen"},
	{domain.DivCEO, "CEO"},
}

// OutputsByDivision routes every deliverable to the division that consumes it
// (the "output" section of the business process). The CEO bucket mirrors the
// whole portfolio. Konsumen and the contractor side of Teknik are fed by the
// per-consumer working-drawing flow.
func (s *Service) OutputsByDivision() []DivisionOutputs {
	projects := s.repo.Projects()
	buckets := map[domain.Division][]OutputItem{}

	add := func(div domain.Division, it OutputItem) {
		buckets[div] = append(buckets[div], it)
	}

	for _, p := range projects {
		for _, t := range p.Tasks {
			it := OutputItem{
				ProjectID: p.ID, ProjectName: p.Name, GP: p.GP,
				Deliverable: t.Name, PIC: t.PIC, Status: t.Status,
				Ready: t.Status == domain.StatusDone,
			}
			// CEO sees the entire deliverable tree.
			add(domain.DivCEO, it)
			if t.Output != domain.DivNone {
				add(t.Output, it)
			}
		}
	}

	// Fold the per-consumer working-drawing flow into Konsumen + Teknik.
	for _, d := range s.repo.WorkDrawings() {
		pname := projectName(projects, d.ProjectID)
		// Consumer working drawing -> Konsumen.
		add(domain.DivKonsumen, OutputItem{
			ProjectID: d.ProjectID, ProjectName: pname, GP: gpFor(projects, d.ProjectID),
			Deliverable: "Gambar kerja konsumen — " + d.Konsumen + " (" + d.Unit + ")",
			PIC:         d.PIC, Status: statusForWD(d, false), Ready: d.KonsumenDone != "",
		})
		// Contractor working drawing -> Teknik.
		add(domain.DivTeknik, OutputItem{
			ProjectID: d.ProjectID, ProjectName: pname, GP: gpFor(projects, d.ProjectID),
			Deliverable: "Gambar kerja kontraktor — " + d.Konsumen + " (" + d.Unit + ")",
			PIC:         d.PIC, Status: statusForWD(d, true), Ready: d.KontraktorDone != "",
		})
	}

	out := make([]DivisionOutputs, 0, len(divisionOrder))
	for _, d := range divisionOrder {
		items := buckets[d.div]
		ready := 0
		for _, it := range items {
			if it.Ready {
				ready++
			}
		}
		out = append(out, DivisionOutputs{
			Division: d.div, Label: d.label, Items: items, Ready: ready, Total: len(items),
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
