// Cross-division roster for the shared department Kanban board. The board is
// usable by ANY division (see transport resolveUserAny), so its member picker
// must offer every employee — not just perencanaan's PIC accounts. The roster
// is pulled from the central auth SSO (GET /departments, then GET
// /dept/{code}/users per department, with the caller's bearer token) and cached
// in memory with a short TTL. It is NEVER written into repo.Users() — that
// store remains the perencanaan PIC account list.
package service

import (
	"sort"
	"strings"
	"sync"
	"time"

	"greenpark/perencanaan/internal/domain"
)

// boardRosterTTL is how long a fetched cross-division roster stays fresh.
const boardRosterTTL = 60 * time.Second

// boardRosterCache holds the last successful cross-division fetch.
type boardRosterCache struct {
	mu    sync.Mutex
	at    time.Time
	users []BoardUser
	depts []domain.Department
}

// crossRoster returns the cross-division roster + department catalogue for the
// board view. Freshness order: valid cache -> fresh fetch -> stale cache ->
// perencanaan-only fallback from the local PIC store. Never fails.
func (s *Service) crossRoster(token string) ([]BoardUser, []domain.Department) {
	s.boardRoster.mu.Lock()
	if len(s.boardRoster.users) > 0 && s.now().Sub(s.boardRoster.at) < boardRosterTTL {
		users, depts := s.boardRoster.users, s.boardRoster.depts
		s.boardRoster.mu.Unlock()
		return users, depts
	}
	s.boardRoster.mu.Unlock()

	// Fetch outside the lock: concurrent duplicate fetches are harmless and a
	// slow auth service must not block every board GET behind one caller.
	if users, depts, ok := s.fetchCrossRoster(token); ok {
		s.boardRoster.mu.Lock()
		s.boardRoster.users, s.boardRoster.depts, s.boardRoster.at = users, depts, s.now()
		s.boardRoster.mu.Unlock()
		return users, depts
	}

	// Fetch failed: serve the last cache if we ever had one.
	s.boardRoster.mu.Lock()
	if len(s.boardRoster.users) > 0 {
		users, depts := s.boardRoster.users, s.boardRoster.depts
		s.boardRoster.mu.Unlock()
		return users, depts
	}
	s.boardRoster.mu.Unlock()

	// Last resort: perencanaan-only roster from the local PIC account store.
	local := s.repo.Users()
	users := make([]BoardUser, 0, len(local))
	for _, u := range local {
		users = append(users, BoardUser{Username: u.Username, Name: u.Name, Role: u.Role, Division: "perencanaan"})
	}
	depts := append([]domain.Department{{Code: "perencanaan", Name: "Perencanaan"}}, s.repo.Departments()...)
	return users, depts
}

// fetchCrossRoster pulls the department catalogue + every department's users
// from auth with the caller's token. ok=false when nothing usable came back
// (auth down, empty token, ...); individual per-department failures are skipped.
func (s *Service) fetchCrossRoster(token string) ([]BoardUser, []domain.Department, bool) {
	if strings.TrimSpace(token) == "" {
		return nil, nil, false
	}
	var raw []authDept
	if err := s.authGetJSON(token, "/departments", &raw); err != nil {
		return nil, nil, false
	}
	depts := make([]domain.Department, 0, len(raw)+1)
	seenDept := map[string]bool{}
	for _, d := range raw {
		code := strings.ToLower(strings.TrimSpace(d.Code))
		if code == "" || seenDept[code] {
			continue
		}
		seenDept[code] = true
		name := strings.TrimSpace(d.Name)
		if name == "" {
			name = code
		}
		depts = append(depts, domain.Department{Code: code, Name: name})
	}
	if !seenDept["perencanaan"] {
		depts = append([]domain.Department{{Code: "perencanaan", Name: "Perencanaan"}}, depts...)
	}

	// Walk perencanaan first (then alphabetical) so an employee holding roles in
	// several departments is listed under perencanaan when applicable.
	codes := make([]string, 0, len(depts))
	for _, d := range depts {
		codes = append(codes, d.Code)
	}
	sort.SliceStable(codes, func(i, j int) bool {
		if codes[i] == "perencanaan" {
			return codes[j] != "perencanaan"
		}
		if codes[j] == "perencanaan" {
			return false
		}
		return codes[i] < codes[j]
	})

	users := []BoardUser{}
	seen := map[string]bool{}
	for _, code := range codes {
		var us []authDeptUser
		if err := s.authGetJSON(token, "/dept/"+code+"/users", &us); err != nil {
			continue // best-effort per department
		}
		for _, au := range us {
			username := strings.ToLower(strings.TrimSpace(au.Username))
			if username == "" || seen[username] {
				continue
			}
			seen[username] = true
			name := strings.TrimSpace(au.Name)
			if name == "" {
				name = username
			}
			role := strings.ToLower(strings.TrimSpace(au.Roles[code]))
			if role == "" {
				role = "staff"
			}
			users = append(users, BoardUser{Username: username, Name: name, Role: role, Division: code})
		}
	}
	if len(users) == 0 {
		return nil, nil, false
	}
	// Local-only PIC accounts (created in perencanaan, absent from SSO) stay
	// assignable on the board too.
	for _, u := range s.repo.Users() {
		if !seen[strings.ToLower(u.Username)] {
			users = append(users, BoardUser{Username: u.Username, Name: u.Name, Role: u.Role, Division: "perencanaan"})
		}
	}
	return users, depts, true
}

// boardRosterHas reports whether username may be added as a card member: a
// local PIC account or anyone on the cross-division roster.
func (s *Service) boardRosterHas(token, username string) bool {
	if _, ok := s.repo.UserByUsername(username); ok {
		return true
	}
	target := strings.ToLower(strings.TrimSpace(username))
	users, _ := s.crossRoster(token)
	for _, u := range users {
		if strings.ToLower(u.Username) == target {
			return true
		}
	}
	return false
}

// knownDeptCodes is the set of department codes a card's Division may take:
// perencanaan itself + the synced catalogue + whatever the roster cache knows.
// Reads only cached data — no network, safe for the PATCH path. Callers must
// NOT hold the repository lock (it reads repo.Departments()).
func (s *Service) knownDeptCodes() map[string]bool {
	set := map[string]bool{"perencanaan": true}
	for _, d := range s.repo.Departments() {
		set[strings.ToLower(d.Code)] = true
	}
	s.boardRoster.mu.Lock()
	for _, d := range s.boardRoster.depts {
		set[strings.ToLower(d.Code)] = true
	}
	s.boardRoster.mu.Unlock()
	return set
}
