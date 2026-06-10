package repository

import (
	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/domain"
)

// seedUsers creates the department accounts: the head of department, the CEO
// overview account, and the three design authors (PIC). Change these in any
// real deployment.
//
// Defaults: ceo/ceo123, kadep/kadep123, randi/randi123, ananto/ananto123,
// agus/agus123.
func seedUsers() map[string]domain.User {
	return map[string]domain.User{
		"ceo":    mustUser("ceo", "Direktur Utama", domain.RoleCEO, "ceo123"),
		"kadep":  mustUser("kadep", "Kepala Departemen Perencanaan", domain.RoleKadep, "kadep123"),
		"randi":  mustUser("randi", "Randi", domain.RoleArsitek, "randi123"),
		"ananto": mustUser("ananto", "Ananto", domain.RoleArsitek, "ananto123"),
		"agus":   mustUser("agus", "Agus", domain.RoleDrafter, "agus123"),
	}
}

func mustUser(username, name, role, password string) domain.User {
	hash, salt, err := auth.HashPassword(password)
	if err != nil {
		panic("seed user " + username + ": " + err.Error())
	}
	return domain.User{Username: username, Name: name, Role: role, Salt: salt, PasswordHash: hash}
}
