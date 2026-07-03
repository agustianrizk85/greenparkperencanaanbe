package service

import (
	"strings"

	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/domain"
)

// Account is a lightweight roster entry for account management (no workload).
type Account struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	RoleLabel string `json:"roleLabel"`
	IsPIC     bool   `json:"isPIC"`
	Fixed     bool   `json:"fixed"` // CEO/Kadep org accounts cannot be deleted
}

// CreateUserInput is the payload to add a design-author (PIC) account at runtime.
type CreateUserInput struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"` // arsitek | drafter
	Password string `json:"password"`
}

// isAssignableRole reports whether a role may be created/removed dynamically.
// CEO and Kadep are the fixed org accounts; only design authors are dynamic.
func isAssignableRole(role string) bool {
	return role == domain.RoleArsitek || role == domain.RoleDrafter
}

// Accounts lists every account for the management view. CEO / Kadep only.
func (s *Service) Accounts(actorRole string) ([]Account, error) {
	if !canManage(actorRole) {
		return nil, ErrForbidden
	}
	out := []Account{}
	for _, u := range s.repo.Users() {
		out = append(out, Account{
			Username:  u.Username,
			Name:      u.Name,
			Role:      u.Role,
			RoleLabel: roleLabels[u.Role],
			IsPIC:     isAssignableRole(u.Role),
			Fixed:     !isAssignableRole(u.Role),
		})
	}
	return out, nil
}

// CreateUser adds a new design-author (PIC) account at runtime. CEO / Kadep only.
// The roster is dynamic: the new account immediately appears as a PIC everywhere
// (summary rollups, staff view, task assignment), with no hardcoded name list.
func (s *Service) CreateUser(actorRole string, in CreateUserInput) (Account, error) {
	if !canManage(actorRole) {
		return Account{}, ErrForbidden
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	name := strings.TrimSpace(in.Name)
	if username == "" || name == "" || strings.TrimSpace(in.Password) == "" {
		return Account{}, ErrValidation
	}
	if !isAssignableRole(in.Role) {
		return Account{}, ErrValidation
	}
	hash, salt, err := auth.HashPassword(in.Password)
	if err != nil {
		return Account{}, err
	}
	u := domain.User{Username: username, Name: name, Role: in.Role, Salt: salt, PasswordHash: hash}
	if !s.repo.AddUser(u) {
		return Account{}, ErrValidation // username already taken
	}
	return Account{Username: u.Username, Name: u.Name, Role: u.Role, RoleLabel: roleLabels[u.Role], IsPIC: true}, nil
}

// DeleteUser removes a dynamic PIC account. CEO / Kadep only; the fixed CEO/Kadep
// org accounts cannot be removed. Tasks previously owned by the account keep
// their pic string and simply show as an unknown/legacy PIC until reassigned.
func (s *Service) DeleteUser(actorRole, username string) error {
	if !canManage(actorRole) {
		return ErrForbidden
	}
	u, ok := s.repo.UserByUsername(username)
	if !ok {
		return ErrNotFound
	}
	if !isAssignableRole(u.Role) {
		return ErrForbidden // CEO/Kadep are fixed
	}
	if !s.repo.DeleteUser(username) {
		return ErrNotFound
	}
	return nil
}
