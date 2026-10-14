package auth

import "time"

// User is a staff member who can log in.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	FullName     string
	Role         string // "admin" or "staff"
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session is a logged-in user, keyed by a random token.
type Session struct {
	Token     string
	UserID    int64
	Username  string
	FullName  string
	Role      string
	ExpiresAt time.Time
}

// IsAdmin reports whether the session belongs to an admin.
func (s *Session) IsAdmin() bool {
	return s.Role == "admin"
}
