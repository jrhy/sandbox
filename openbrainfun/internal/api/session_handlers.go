package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/security"
)

type AuthService interface {
	AuthenticatePassword(ctx context.Context, username, password string) (auth.Session, error)
	LogoutSession(ctx context.Context, token string) error
}

type SessionHandlers struct {
	authService  AuthService
	cookieSecure bool
	csrfKey      string
}

type postSessionRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type postSessionResponse struct {
	CSRFToken string `json:"csrf_token"`
}

func NewSessionHandlers(authService AuthService, cookieSecure bool, csrfKey string) *SessionHandlers {
	return &SessionHandlers{authService: authService, cookieSecure: cookieSecure, csrfKey: csrfKey}
}

func (h *SessionHandlers) PostSession(w http.ResponseWriter, r *http.Request) {
	var req postSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	session, err := h.authService.AuthenticatePassword(r.Context(), req.Username, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			status = http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
		return
	}

	http.SetCookie(w, auth.NewSessionCookie(session.Token, session.ExpiresAt, h.cookieSecure))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(postSessionResponse{CSRFToken: security.Token(h.csrfKey, session.Token)})
}

func (h *SessionHandlers) DeleteSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" && h.authService != nil {
		_ = h.authService.LogoutSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, auth.ExpiredSessionCookie(h.cookieSecure))
	w.WriteHeader(http.StatusNoContent)
}
