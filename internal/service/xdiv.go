// Cross-division read surface: lets OTHER department modules (e.g. Legal Permit)
// discover + link a perencanaan project and pull the deliverables routed to their
// division — most importantly the Siteplan (tasks with Output=legalpermit). Read
// only; served over the board submux which already accepts ANY division's SSO
// token (resolveUserAny). The deliverable DOCUMENT itself is downloaded through
// the existing board task-doc endpoint (/api/board/task/{pid}/{tid}/doc).
package service

import "greenpark/perencanaan/internal/domain"

// XDivProject is a minimal project entry for cross-division linking.
type XDivProject struct {
	ID     string `json:"id"`
	GP     string `json:"gp"`
	Name   string `json:"name"`
	Lokasi string `json:"lokasi"`
}

// XDivDeliverable is one deliverable routed to a division, exposed cross-division.
type XDivDeliverable struct {
	ProjectID   string                  `json:"projectId"`
	ProjectName string                  `json:"projectName"`
	GP          string                  `json:"gp"`
	TaskID      string                  `json:"taskId"`
	Category    string                  `json:"category"`
	Group       string                  `json:"group"`
	Deliverable string                  `json:"deliverable"`
	PIC         string                  `json:"pic"`
	Output      domain.Division         `json:"output"`
	Status      domain.TaskStatus       `json:"status"`
	HasDoc      bool                    `json:"hasDoc"`
	ApprovedBy  string                  `json:"approvedBy"`
	UpdatedAt   string                  `json:"updatedAt"`
	Attachments []domain.TaskAttachment `json:"attachments,omitempty"`
}

// XDivProjects returns every project as a minimal {id,gp,name} for a linker.
func (s *Service) XDivProjects() []XDivProject {
	out := []XDivProject{}
	for _, p := range s.repo.Projects() {
		out = append(out, XDivProject{ID: p.ID, GP: p.GP, Name: p.Name, Lokasi: p.Lokasi})
	}
	return out
}

// XDivUnit is one kavling (unit) flattened with project + type names resolved,
// for a cross-division read (e.g. Teknik "Master Unit" — perencanaan is owner).
type XDivUnit struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	ProjectName  string `json:"projectName"`
	GP           string `json:"gp"`
	Blok         string `json:"blok"`
	NoKav        string `json:"noKav"`
	Type         string `json:"type"`
	LuasBangunan int    `json:"luasBangunan"`
	Lebar        string `json:"lebar"`
}

// XDivUnits returns every kavling across all projects, flattened, with the blok
// + building-type names resolved. Read-only cross-division surface.
func (s *Service) XDivUnits() []XDivUnit {
	out := []XDivUnit{}
	typeName := map[string]string{}
	for _, t := range s.repo.BuildingTypes() {
		typeName[t.ID] = t.Name
	}
	for _, p := range s.repo.Projects() {
		blokName := map[string]string{}
		for _, b := range s.repo.BloksByProject(p.ID) {
			blokName[b.ID] = b.Name
		}
		for _, k := range s.repo.KavlingByProject(p.ID) {
			out = append(out, XDivUnit{
				ID:           k.ID,
				ProjectID:    p.ID,
				ProjectName:  p.Name,
				GP:           p.GP,
				Blok:         blokName[k.BlokID],
				NoKav:        k.NoKav,
				Type:         typeName[k.TypeID],
				LuasBangunan: k.LuasBangunan,
				Lebar:        k.LebarKavling,
			})
		}
	}
	return out
}

// XDivDeliverables returns deliverables whose Output == division (e.g.
// "legalpermit" → the Siteplan tasks). When projectID is non-empty it is scoped
// to that project. Empty division returns all division-routed deliverables.
func (s *Service) XDivDeliverables(projectID string, division domain.Division) []XDivDeliverable {
	out := []XDivDeliverable{}
	for _, p := range s.repo.Projects() {
		if projectID != "" && p.ID != projectID {
			continue
		}
		for _, t := range p.Tasks {
			if division != "" {
				if t.Output != division {
					continue
				}
			} else if t.Output == domain.DivNone {
				continue
			}
			out = append(out, XDivDeliverable{
				ProjectID: p.ID, ProjectName: p.Name, GP: p.GP,
				TaskID: t.ID, Category: t.Category, Group: t.Group, Deliverable: t.Name,
				PIC: t.PIC, Output: t.Output, Status: t.Status,
				HasDoc: t.Doc != nil, ApprovedBy: t.ApprovedBy, UpdatedAt: t.UpdatedAt,
				Attachments: t.Attachments,
			})
		}
	}
	return out
}
