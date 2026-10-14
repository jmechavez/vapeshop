package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const cookieName = "dc_session"

// Session duration: 12 hours.
const sessionTTL = 12 * time.Hour

// Store keeps sessions in memory.
// Simple, correct, and disappears on server restart (users just log in again).
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewStore() *Store {
	return &Store{sessions: make(map[string]*Session)}
}

// Create generates a token, stores the session, returns the token.
func (s *Store) Create(u *User) string {
	token := newToken()
	sess := &Session{
		Token:     token,
		UserID:    u.ID,
		Username:  u.Username,
		FullName:  u.FullName,
		Role:      u.Role,
		ExpiresAt: time.Now().Add(sessionTTL),
	}

	s.mu.Lock()
	s.sessions[token] = sess
	s.mu.Unlock()

	return token
}

// Get returns the session if it exists and hasn't expired.
func (s *Store) Get(token string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.Delete(token)
		return nil, false
	}
	return sess, true
}

// Delete removes a session (used on logout).
func (s *Store) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to read random bytes: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// --- cookie helpers ---

// SetCookie writes the session cookie to the response.
func SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
		// Secure: true, // enable when serving over HTTPS in production
	})
}

// ClearCookie removes the session cookie.
func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// TokenFromRequest reads the session token from the request cookie.
func TokenFromRequest(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
