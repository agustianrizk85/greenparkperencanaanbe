package service

import (
	"math"

	"greenpark/perencanaan/internal/domain"
)

// PICLoad is the workload of one design author across the whole portfolio.
type PICLoad struct {
	PIC      string `json:"pic"`
	Total    int    `json:"total"`
	Done     int    `json:"done"`
	Progress int    `json:"progress"` // 0-100, weighted
}

// DivisionStat is the ready/total deliverable count for one division.
type DivisionStat struct {
	Division domain.Division `json:"division"`
	Label    string          `json:"label"`
	Ready    int             `json:"ready"`
	Total    int             `json:"total"`
}

// AlertCounts breaks alerts down by severity.
type AlertCounts struct {
	Red   int `json:"red"`
	Amber int `json:"amber"`
	Green int `json:"green"`
}

// Summary is the top-of-dashboard portfolio snapshot.
type Summary struct {
	Today       string         `json:"today"`
	Projects    int            `json:"projects"`
	AvgProgress int            `json:"avgProgress"`
	Tasks       int            `json:"tasks"`
	TasksDone   int            `json:"tasksDone"`
	PICs        []PICLoad      `json:"pics"`
	Divisions   []DivisionStat `json:"divisions"`
	Alerts      AlertCounts    `json:"alerts"`
}

// picOrder fixes the display order of the three authors.
var picOrder = []string{domain.PicRandi, domain.PicAnanto, domain.PicAgus}

// Summary aggregates the portfolio into the headline metrics.
func (s *Service) Summary() Summary {
	projects := s.repo.Projects()

	sum := Summary{Today: s.today(), Projects: len(projects)}

	loads := map[string]*PICLoad{}
	for _, pic := range picOrder {
		loads[pic] = &PICLoad{PIC: pic}
	}

	var progressSum float64
	picWeight := map[string]float64{}
	for _, p := range projects {
		sum.Tasks += len(p.Tasks)
		progressSum += float64(rollupProject(p, false).Progress)
		for _, t := range p.Tasks {
			if t.Status == domain.StatusDone {
				sum.TasksDone++
			}
			l, ok := loads[t.PIC]
			if !ok {
				l = &PICLoad{PIC: t.PIC}
				loads[t.PIC] = l
			}
			l.Total++
			if t.Status == domain.StatusDone {
				l.Done++
			}
			picWeight[t.PIC] += t.Status.Weight()
		}
	}
	if len(projects) > 0 {
		sum.AvgProgress = int(math.Round(progressSum / float64(len(projects))))
	}
	for pic, l := range loads {
		if l.Total > 0 {
			l.Progress = int(math.Round(picWeight[pic] / float64(l.Total) * 100))
		}
	}
	for _, pic := range picOrder {
		if l, ok := loads[pic]; ok {
			sum.PICs = append(sum.PICs, *l)
		}
	}

	for _, d := range s.OutputsByDivision() {
		sum.Divisions = append(sum.Divisions, DivisionStat{
			Division: d.Division, Label: d.Label, Ready: d.Ready, Total: d.Total,
		})
	}

	for _, a := range s.Alerts() {
		switch a.Sev {
		case RagRed:
			sum.Alerts.Red++
		case RagAmber:
			sum.Alerts.Amber++
		default:
			sum.Alerts.Green++
		}
	}
	return sum
}
