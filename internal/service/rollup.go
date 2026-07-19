package service

import (
	"math"

	"greenpark/perencanaan/internal/domain"
)

// Rag is a traffic-light status used across the dashboard.
type Rag string

const (
	RagGreen Rag = "green"
	RagAmber Rag = "amber"
	RagRed   Rag = "red"
	RagGrey  Rag = "grey"
)

// GroupRollup is the progress of one mid-level deliverable (e.g. "Denah").
type GroupRollup struct {
	Group    string `json:"group"`
	Progress int    `json:"progress"` // 0-100
	Done     int    `json:"done"`
	Total    int    `json:"total"`
}

// CategoryRollup is the progress of one top-level category.
type CategoryRollup struct {
	Category string        `json:"category"`
	Progress int           `json:"progress"`
	Groups   []GroupRollup `json:"groups"`
}

// ProjectRollup is a project summary plus derived progress. Categories is only
// populated for the detail view.
type ProjectRollup struct {
	ID         string           `json:"id"`
	No         int              `json:"no"`
	GP         string           `json:"gp"`
	Name       string           `json:"name"`
	Lokasi     string           `json:"lokasi"`
	Luas       string           `json:"luas"`
	Units      int              `json:"units"`
	Types      int              `json:"types"`
	Progress   int              `json:"progress"` // 0-100, weighted by status
	Status     Rag              `json:"status"`
	Done       int              `json:"done"`  // tasks fully done
	Total      int              `json:"total"` // total tasks
	Categories []CategoryRollup `json:"categories,omitempty"`
}

// ProjectDetail is a project rollup with its full task list.
type ProjectDetail struct {
	ProjectRollup
	Tasks   []domain.Task    `json:"tasks"`
	Bloks   []domain.Blok    `json:"bloks"`
	Kavling []domain.Kavling `json:"kavling"`
}

// AssignedTask is a task annotated with its owning project (for the PIC view).
type AssignedTask struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	GP          string `json:"gp"`
	domain.Task
}

// rollupProject computes progress for a project. When withCategories is true the
// per-category / per-group breakdown is included (detail view).
func rollupProject(p domain.Project, withCategories bool) ProjectRollup {
	r := ProjectRollup{
		ID: p.ID, No: p.No, GP: p.GP, Name: p.Name, Lokasi: p.Lokasi,
		Luas: p.Luas, Units: p.Units, Types: p.Types, Total: len(p.Tasks),
	}

	var weightSum float64
	for _, t := range p.Tasks {
		weightSum += t.Status.Weight()
		if t.Status == domain.StatusDone {
			r.Done++
		}
	}
	if r.Total > 0 {
		r.Progress = int(math.Round(weightSum / float64(r.Total) * 100))
	}
	r.Status = ragForProgress(r.Progress)

	if withCategories {
		r.Categories = categoryRollups(p.Tasks)
	}
	return r
}

// categoryRollups groups tasks by category then group, preserving the canonical
// category order and first-seen group order.
func categoryRollups(tasks []domain.Task) []CategoryRollup {
	// Accumulate weighted progress per (category, group).
	type acc struct {
		weight float64
		done   int
		total  int
	}
	catGroups := map[string]map[string]*acc{} // category -> group -> acc
	groupOrder := map[string][]string{}       // category -> ordered group names

	for _, t := range tasks {
		g, ok := catGroups[t.Category]
		if !ok {
			g = map[string]*acc{}
			catGroups[t.Category] = g
		}
		a, ok := g[t.Group]
		if !ok {
			a = &acc{}
			g[t.Group] = a
			groupOrder[t.Category] = append(groupOrder[t.Category], t.Group)
		}
		a.weight += t.Status.Weight()
		a.total++
		if t.Status == domain.StatusDone {
			a.done++
		}
	}

	out := []CategoryRollup{}
	for _, cat := range domain.CategoryOrder {
		groups, ok := catGroups[cat]
		if !ok {
			continue
		}
		cr := CategoryRollup{Category: cat}
		var catWeight float64
		var catTotal int
		for _, gname := range groupOrder[cat] {
			a := groups[gname]
			prog := 0
			if a.total > 0 {
				prog = int(math.Round(a.weight / float64(a.total) * 100))
			}
			cr.Groups = append(cr.Groups, GroupRollup{
				Group: gname, Progress: prog, Done: a.done, Total: a.total,
			})
			catWeight += a.weight
			catTotal += a.total
		}
		if catTotal > 0 {
			cr.Progress = int(math.Round(catWeight / float64(catTotal) * 100))
		}
		out = append(out, cr)
	}
	return out
}

func ragForProgress(p int) Rag {
	switch {
	case p >= 100:
		return RagGreen
	case p <= 0:
		return RagGrey
	default:
		return RagAmber
	}
}
