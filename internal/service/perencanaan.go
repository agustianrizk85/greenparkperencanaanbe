// Package service holds the business logic of the Perencanaan (planning) Design
// Readiness Control Tower. The dashboard data is a snapshot served verbatim; the
// readiness model (pipeline, gates, capacity, alerts) is derived on the client
// from this raw data. The service therefore exposes the raw data plus the
// authentication use-cases.
package service

import (
	"errors"

	"greenpark/perencanaan/internal/auth"
	"greenpark/perencanaan/internal/domain"
	"greenpark/perencanaan/internal/repository"
)

// Sentinel errors mapped to HTTP status codes by the transport layer.
var (
	ErrNotFound           = errors.New("resource not found")
	ErrValidation         = errors.New("validation failed")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// Service exposes the planning data read and the auth use-cases.
type Service struct {
	repo     *repository.Memory
	sessions *auth.SessionStore
}

// New builds a Service from the store and session manager.
func New(repo *repository.Memory, sessions *auth.SessionStore) *Service {
	return &Service{repo: repo, sessions: sessions}
}

// Data returns the raw planning JSON snapshot (today, projects, units, codeMap).
func (s *Service) Data() []byte { return s.repo.Data() }

/* ---- Auth -------------------------------------------------------------- */

// Login verifies credentials and issues a bearer token.
func (s *Service) Login(username, password string) (string, domain.User, error) {
	u, ok := s.repo.UserByUsername(username)
	if !ok || !auth.Verify(password, u.PasswordHash, u.Salt) {
		return "", domain.User{}, ErrInvalidCredentials
	}
	token, err := s.sessions.Issue(u.Username)
	if err != nil {
		return "", domain.User{}, err
	}
	return token, u, nil
}

// UserByToken resolves the user behind a valid session token.
func (s *Service) UserByToken(token string) (domain.User, bool) {
	username, ok := s.sessions.Resolve(token)
	if !ok {
		return domain.User{}, false
	}
	return s.repo.UserByUsername(username)
}

// Logout revokes a session token.
func (s *Service) Logout(token string) { s.sessions.Revoke(token) }
