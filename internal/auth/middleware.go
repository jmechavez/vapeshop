package auth

import (
	"context"
	"net/http"
)

type ctxKey int

const sessionKey ctxKey = 1

// RequireLogin redirects to /login if there is no valid session.
func (s *Store) RequireLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := TokenFromRequest(r)
		sess, ok := s.Get(token)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), sessionKey, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin must be used inside RequireLogin (after a session exists).
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := FromContext(r.Context())
		if sess == nil || !sess.IsAdmin() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// FromContext returns the session attached by RequireLogin, or nil.
func FromContext(ctx context.Context) *Session {
	sess, _ := ctx.Value(sessionKey).(*Session)
	return sess
}
