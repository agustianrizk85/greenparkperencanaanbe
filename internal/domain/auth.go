package domain

// User is a dashboard operator who can sign in and manage master data.
// The password material is never serialised to JSON.
type User struct {
	Username     string `json:"username"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Salt         []byte `json:"-"`
	PasswordHash []byte `json:"-"`
}
