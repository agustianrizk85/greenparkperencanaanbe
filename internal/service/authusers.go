// Account roster sync from the central auth SSO service. Employees are managed
// ONCE in the Admin pusat (auth); perencanaan pulls the department roster from
// GET /dept/perencanaan/users so newly added karyawan appear here as assignable
// PIC automatically — no separate account list to maintain.
package service

import (
	"strings"

	"greenpark/perencanaan/internal/domain"
)

// authDeptUser is the shape returned by auth's GET /api/dept/{dept}/users.
type authDeptUser struct {
	Username string            `json:"username"`
	Name     string            `json:"name"`
	Roles    map[string]string `json:"roles"`
}

// authDept is the shape returned by auth's GET /api/departments.
type authDept struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// syncFromAuth refreshes BOTH the PIC roster and the department catalogue from
// the central auth SSO (best-effort). Called wherever perencanaan needs a fresh
// roster/division list (Data Master, Tim).
func (s *Service) syncFromAuth(token string) {
	s.syncUsersFromAuth(token)
	s.syncDepartmentsFromAuth(token)
}

// syncDepartmentsFromAuth pulls the central department catalogue and caches it as
// the "output to division" options — every department EXCEPT perencanaan itself
// (a deliverable does not output back to its own division).
func (s *Service) syncDepartmentsFromAuth(token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	var depts []authDept
	if err := s.authGetJSON(token, "/departments", &depts); err != nil {
		return
	}
	out := make([]domain.Department, 0, len(depts))
	for _, d := range depts {
		code := strings.ToLower(strings.TrimSpace(d.Code))
		if code == "" || code == "perencanaan" {
			continue
		}
		name := strings.TrimSpace(d.Name)
		if name == "" {
			name = code
		}
		out = append(out, domain.Department{Code: code, Name: name})
	}
	if len(out) > 0 {
		s.repo.SetDepartments(out)
	}
}

// syncUsersFromAuth pulls the perencanaan department roster from the central auth
// service and upserts the OPERATIONAL members (Kadep + design authors) into the
// local store, so PIC lists / assignment / staff view stay in sync with SSO.
// Best-effort: any error (auth down, no token) leaves the existing roster intact.
func (s *Service) syncUsersFromAuth(token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	var users []authDeptUser
	if err := s.authGetJSON(token, "/dept/perencanaan/users", &users); err != nil {
		return
	}
	for _, au := range users {
		role := strings.ToLower(strings.TrimSpace(au.Roles["perencanaan"]))
		// Only operational perencanaan roles become local accounts. Directors
		// (ceo/dirops) and admin/viewer map via SSO on the fly and are not PICs.
		switch role {
		case domain.RoleKadep, domain.RoleArsitek, domain.RoleDrafter:
		default:
			continue
		}
		username := strings.ToLower(strings.TrimSpace(au.Username))
		name := strings.TrimSpace(au.Name)
		if username == "" || name == "" {
			continue
		}
		s.repo.UpsertUser(domain.User{Username: username, Name: name, Role: role})
	}
}
