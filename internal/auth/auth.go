// Package auth provides password hashing (PBKDF2-HMAC-SHA256, stdlib only) and
// an in-memory, token-based session store. No external dependencies.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

const (
	iterations = 100_000
	keyLength  = 32
	saltLength = 16
)

// HashPassword returns a random salt and the PBKDF2 hash of the password.
func HashPassword(password string) (hash, salt []byte, err error) {
	salt = make([]byte, saltLength)
	if _, err = rand.Read(salt); err != nil {
		return nil, nil, err
	}
	hash, err = pbkdf2.Key(sha256.New, password, salt, iterations, keyLength)
	if err != nil {
		return nil, nil, err
	}
	return hash, salt, nil
}

// Verify reports whether password matches the stored hash/salt (constant time).
func Verify(password string, hash, salt []byte) bool {
	candidate, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyLength)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(candidate, hash) == 1
}

type session struct {
	username string
	expires  time.Time
}

// SessionStore holds opaque bearer tokens mapped to usernames with a TTL.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
	ttl      time.Duration
}

// NewSessionStore creates a session store with the given token lifetime.
func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{sessions: make(map[string]session), ttl: ttl}
}

// Issue creates and stores a new random token for the username.
func (s *SessionStore) Issue(username string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessions[token] = session{username: username, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return token, nil
}

// Resolve returns the username for a valid, non-expired token.
func (s *SessionStore) Resolve(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return "", false
	}
	if time.Now().After(sess.expires) {
		delete(s.sessions, token)
		return "", false
	}
	return sess.username, true
}

// Revoke deletes a token (logout).
func (s *SessionStore) Revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}
