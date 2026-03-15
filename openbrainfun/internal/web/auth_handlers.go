package web

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"net/http"

	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

type AuthService interface {
	AuthenticatePassword(ctx context.Context, username, password string) (auth.Session, error)
	LogoutSession(ctx context.Context, token string) error
}

type AuthHandlers struct {
	authService   AuthService
	cookieSecure  bool
	templateLogin *template.Template
}

type loginPageData struct {
	Error string
}

func NewAuthHandlers(authService AuthService, cookieSecure bool, csrfKey string) *AuthHandlers {
	return &AuthHandlers{
		authService:   authService,
		cookieSecure:  cookieSecure,
		templateLogin: template.Must(template.ParseFS(templateFS, "templates/login.tmpl")),
	}
}

func (h *AuthHandlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templateLogin.Execute(w, loginPageData{}); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *AuthHandlers) PostLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	session, err := h.authService.AuthenticatePassword(r.Context(), r.PostForm.Get("username"), r.PostForm.Get("password"))
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
		if renderErr := h.templateLogin.Execute(w, loginPageData{Error: "Invalid username or password."}); renderErr != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	http.SetCookie(w, auth.NewSessionCookie(session.Token, session.ExpiresAt, h.cookieSecure))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *AuthHandlers) PostLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" && h.authService != nil {
		_ = h.authService.LogoutSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, auth.ExpiredSessionCookie(h.cookieSecure))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
