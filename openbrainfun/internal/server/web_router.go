package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/api"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/config"
	"github.com/jrhy/sandbox/openbrainfun/internal/security"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
	"github.com/jrhy/sandbox/openbrainfun/internal/web"
)

type WebAuthService interface {
	web.AuthService
	api.AuthService
	RequireSession(ctx context.Context, token string) (auth.User, auth.Session, error)
}

type WebThoughtHandler interface {
	Index(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Thoughts(w http.ResponseWriter, r *http.Request)
	ThoughtDetail(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	DeleteConfirm(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Retry(w http.ResponseWriter, r *http.Request)
}

type APIThoughtHandler interface {
	Create(w http.ResponseWriter, r *http.Request)
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Related(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Retry(w http.ResponseWriter, r *http.Request)
}

func NewWebRouter(cfg config.Config, authService WebAuthService, webHandlers *web.AuthHandlers, apiHandlers *api.SessionHandlers, webThoughtHandlers WebThoughtHandler, apiThoughtHandlers APIThoughtHandler) http.Handler {
	if webHandlers == nil {
		webHandlers = web.NewAuthHandlers(authService, cfg.CookieSecure, cfg.CSRFKey)
	}
	if apiHandlers == nil {
		apiHandlers = api.NewSessionHandlers(authService, cfg.CookieSecure, cfg.CSRFKey)
	}
	if webThoughtHandlers == nil {
		webThoughtHandlers = web.NewHandlers(noopThoughtService{}, cfg.CSRFKey)
	}
	if apiThoughtHandlers == nil {
		apiThoughtHandlers = api.NewThoughtHandlers(noopThoughtService{})
	}

	csrfProtection := security.NewCSRFMiddleware(cfg.CSRFKey)
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			webHandlers.LoginPage(w, r)
		case http.MethodPost:
			webHandlers.PostLogin(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	})
	mux.Handle("/logout", requireBrowserAuth(authService, csrfProtection(http.HandlerFunc(webHandlers.PostLogout))))
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			apiHandlers.PostSession(w, r)
		case http.MethodDelete:
			apiHandlers.DeleteSession(w, r)
		default:
			w.Header().Set("Allow", "POST, DELETE")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	})
	mux.Handle("/api/thoughts", requireAPIAuth(authService, csrfProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiThoughtHandlers.List(w, r)
		case http.MethodPost:
			apiThoughtHandlers.Create(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}))))
	mux.Handle("/api/thoughts/", requireAPIAuth(authService, csrfProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case hasPathSuffix(r.URL.Path, "/retry"):
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			apiThoughtHandlers.Retry(w, r)
		case hasPathSuffix(r.URL.Path, "/related"):
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			apiThoughtHandlers.Related(w, r)
		case r.Method == http.MethodGet:
			apiThoughtHandlers.Get(w, r)
		case r.Method == http.MethodPatch:
			apiThoughtHandlers.Update(w, r)
		case r.Method == http.MethodDelete:
			apiThoughtHandlers.Delete(w, r)
		default:
			w.Header().Set("Allow", "GET, PATCH, DELETE, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}))))
	mux.Handle("/", requireBrowserAuth(authService, csrfProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		webThoughtHandlers.Index(w, r)
	}))))
	mux.Handle("/thoughts", requireBrowserAuth(authService, csrfProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			webThoughtHandlers.Thoughts(w, r)
		case http.MethodPost:
			webThoughtHandlers.Create(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}))))
	mux.Handle("/thoughts/", requireBrowserAuth(authService, csrfProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case hasPathSuffix(r.URL.Path, "/delete"):
			switch r.Method {
			case http.MethodGet:
				webThoughtHandlers.DeleteConfirm(w, r)
			case http.MethodPost:
				webThoughtHandlers.Delete(w, r)
			default:
				w.Header().Set("Allow", "GET, POST")
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			}
		case hasPathSuffix(r.URL.Path, "/retry"):
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			webThoughtHandlers.Retry(w, r)
		case r.Method == http.MethodGet:
			webThoughtHandlers.ThoughtDetail(w, r)
		case r.Method == http.MethodPost:
			webThoughtHandlers.Update(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}))))

	return mux
}

func requireBrowserAuth(authService WebAuthService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := authenticatedContext(r.Context(), authService, r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireAPIAuth(authService WebAuthService, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := authenticatedContext(r.Context(), authService, r)
		if !ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authenticatedContext(ctx context.Context, authService WebAuthService, r *http.Request) (context.Context, bool) {
	if authService == nil {
		return nil, false
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	user, _, err := authService.RequireSession(ctx, cookie.Value)
	if err != nil {
		return nil, false
	}
	ctx = context.WithValue(ctx, auth.ContextUserKey{}, user)
	ctx = context.WithValue(ctx, auth.ContextSessionTokenKey{}, cookie.Value)
	return ctx, true
}

func hasPathSuffix(path, suffix string) bool {
	return len(path) > len(suffix) && len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

type noopThoughtService struct{}

func (noopThoughtService) CreateThought(ctx context.Context, input thoughts.CreateThoughtInput) (thoughts.Thought, error) {
	return thoughts.Thought{}, errors.New("thought service unavailable")
}

func (noopThoughtService) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	return thoughts.Thought{}, thoughts.ErrThoughtNotFound
}

func (noopThoughtService) UpdateThought(ctx context.Context, input thoughts.UpdateThoughtInput) (thoughts.Thought, error) {
	return thoughts.Thought{}, errors.New("thought service unavailable")
}

func (noopThoughtService) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	return thoughts.ErrThoughtNotFound
}

func (noopThoughtService) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	return []thoughts.Thought{}, nil
}

func (noopThoughtService) SearchThoughts(ctx context.Context, input thoughts.SearchThoughtsInput) ([]thoughts.ScoredThought, error) {
	return []thoughts.ScoredThought{}, nil
}

func (noopThoughtService) RelatedThoughts(ctx context.Context, input thoughts.RelatedThoughtsInput) ([]thoughts.ScoredThought, error) {
	return []thoughts.ScoredThought{}, nil
}

func (noopThoughtService) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	return thoughts.Thought{}, thoughts.ErrThoughtNotFound
}
