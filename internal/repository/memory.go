// Package repository holds the planning data store. The dashboard data set is a
// snapshot exported from the department's Excel monitors (Progres Monitor +
// Data Master Proyek), embedded at build time and served verbatim. User
// accounts for authentication live alongside it in memory.
package repository

import (
	_ "embed"

	"greenpark/perencanaan/internal/domain"
)

// planningData is the raw JSON snapshot (today, projects, units, codeMap),
// embedded so it ships inside the binary and is served byte-for-byte.
//
//go:embed data.json
var planningData []byte

// Memory is the in-memory store: the embedded planning snapshot plus the user
// accounts used for login.
type Memory struct {
	users map[string]domain.User
}

// NewMemory builds the store with the seeded user accounts.
func NewMemory() *Memory {
	return &Memory{users: seedUsers()}
}

// Data returns the raw planning JSON snapshot.
func (m *Memory) Data() []byte { return planningData }

// UserByUsername looks up an account for authentication.
func (m *Memory) UserByUsername(username string) (domain.User, bool) {
	u, ok := m.users[username]
	return u, ok
}
