package service

import "greenpark/perencanaan/internal/domain"

// MasterProjectInfo is a project's MASTER identity only — no progress / status
// (that is process data shown elsewhere).
type MasterProjectInfo struct {
	ID     string `json:"id"`
	No     int    `json:"no"`
	GP     string `json:"gp"`
	Name   string `json:"name"`
	Lokasi string `json:"lokasi"`
	Luas   string `json:"luas"`
	Units  int    `json:"units"`
	Types  int    `json:"types"`
	Tasks  int    `json:"tasks"` // number of deliverables in the project tree
	Added  bool   `json:"added"` // true if added manually (beyond the seeded 32)
}

// DivisionInfo is one downstream output division (master reference).
type DivisionInfo struct {
	Division domain.Division `json:"division"`
	Label    string          `json:"label"`
}

// AccountInfo is a user account's MASTER identity (no workload).
type AccountInfo struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	RoleLabel string `json:"roleLabel"`
	IsPIC     bool   `json:"isPic"`
}

// MasterData bundles every piece of MASTER reference data behind the "Data
// Master" tab: the project portfolio identity, the fixed deliverable template,
// the accounts/roles, and the output divisions. None of this carries process
// state.
type MasterData struct {
	Projects  []MasterProjectInfo       `json:"projects"`
	Template  []domain.TemplateCategory `json:"template"`
	Accounts  []AccountInfo             `json:"accounts"`
	Divisions []DivisionInfo            `json:"divisions"`
	SeedCount int                       `json:"seedCount"` // number of seeded (built-in) projects
}

// seedProjectCount is the size of the built-in portfolio (projects.json). Used
// to flag manually added projects in the master view.
const seedProjectCount = 32

// Master returns all master reference data for the "Data Master" view.
func (s *Service) Master() MasterData {
	projects := s.repo.Projects()
	md := MasterData{
		Template:  domain.TemplateTree(),
		SeedCount: seedProjectCount,
	}
	for _, p := range projects {
		md.Projects = append(md.Projects, MasterProjectInfo{
			ID: p.ID, No: p.No, GP: p.GP, Name: p.Name, Lokasi: p.Lokasi,
			Luas: p.Luas, Units: p.Units, Types: p.Types, Tasks: len(p.Tasks),
			Added: p.No > seedProjectCount,
		})
	}
	for _, u := range s.repo.Users() {
		isPIC := u.Role == domain.RoleArsitek || u.Role == domain.RoleDrafter
		md.Accounts = append(md.Accounts, AccountInfo{
			Username: u.Username, Name: u.Name, Role: u.Role,
			RoleLabel: roleLabels[u.Role], IsPIC: isPIC,
		})
	}
	for _, d := range divisionOrder {
		md.Divisions = append(md.Divisions, DivisionInfo{Division: d.div, Label: d.label})
	}
	return md
}
