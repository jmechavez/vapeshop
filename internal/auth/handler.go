package auth

import (
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	Store *Store

	// RenderLogin is set by main.go so this package doesn't import views/pages.
	RenderLogin func(w http.ResponseWriter, r *http.Request, errMsg string)
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.Store.Get(TokenFromRequest(r)); ok {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	h.RenderLogin(w, r, "")
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	u, err := FindByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			h.RenderLogin(w, r, "Invalid username or password")
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	if !u.Active {
		h.RenderLogin(w, r, "This account is disabled")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		h.RenderLogin(w, r, "Invalid username or password")
		return
	}

	token := h.Store.Create(u)
	SetCookie(w, token)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token := TokenFromRequest(r)
	if token != "" {
		h.Store.Delete(token)
	}
	ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
