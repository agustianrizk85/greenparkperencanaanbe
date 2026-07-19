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
	GPs       []domain.GP               `json:"gps"`
	Types     []domain.BuildingType     `json:"types"`
	Lebars    []domain.Lebar            `json:"lebars"`
	Lokasis   []domain.Lokasi           `json:"lokasis"`
	SeedCount int                       `json:"seedCount"` // number of seeded (built-in) projects
}

// seedProjectCount is the size of the built-in portfolio (projects.json). Used
// to flag manually added projects in the master view.
const seedProjectCount = 32

// Master returns all master reference data for the "Data Master" view. It first
// syncs the account roster from the central auth SSO (best-effort) so karyawan
// added in Admin pusat appear here as assignable PIC.
func (s *Service) Master(token string) MasterData {
	s.syncFromAuth(token)
	projects := s.repo.Projects()
	// Initialise slices so an empty portfolio serialises as [] (not null), which
	// the frontend maps over directly.
	md := MasterData{
		Template:  domain.TemplateTree(),
		SeedCount: seedProjectCount,
		Projects:  []MasterProjectInfo{},
		Accounts:  []AccountInfo{},
		Divisions: []DivisionInfo{},
		GPs:       s.repo.GPs(),
		Types:     s.repo.BuildingTypes(),
		Lebars:    s.repo.Lebars(),
		Lokasis:   s.repo.Lokasis(),
	}
	for _, p := range projects {
		// Jumlah Unit/Tipe are DERIVED from kavling (Fase 2), not manual counts.
		kav := s.repo.KavlingByProject(p.ID)
		typeSet := map[string]bool{}
		for _, k := range kav {
			if k.TypeID != "" {
				typeSet[k.TypeID] = true
			}
		}
		md.Projects = append(md.Projects, MasterProjectInfo{
			ID: p.ID, No: p.No, GP: p.GP, Name: p.Name, Lokasi: p.Lokasi,
			Luas: p.Luas, Units: len(kav), Types: len(typeSet), Tasks: len(p.Tasks),
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
	for _, d := range s.repo.Departments() {
		md.Divisions = append(md.Divisions, DivisionInfo{Division: domain.Division(d.Code), Label: d.Name})
	}
	return md
}
