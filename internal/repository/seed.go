package repository

import (
	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/domain"
)

// seedUsers creates the default accounts. Change these in any real deployment.
// Default credentials: admin/admin123 (Kadep) and viewer/viewer123 (read-only).
func seedUsers() map[string]domain.User {
	return map[string]domain.User{
		"admin":  mustUser("admin", "Administrator Perencanaan", "admin", "admin123"),
		"viewer": mustUser("viewer", "Viewer", "viewer", "viewer123"),
	}
}

func mustUser(username, name, role, password string) domain.User {
	hash, salt, err := auth.HashPassword(password)
	if err != nil {
		panic("seed user " + username + ": " + err.Error())
	}
	return domain.User{Username: username, Name: name, Role: role, Salt: salt, PasswordHash: hash}
}
