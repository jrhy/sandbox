package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/api"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/config"
	"github.com/jrhy/sandbox/openbrainfun/internal/security"
	"github.com/jrhy/sandbox/openbrainfun/internal/web"
)

func TestProtectedRouteRedirectsToLogin(t *testing.T) {
	router := NewWebRouter(config.Config{CookieSecure: false, CSRFKey: "test-csrf-key"}, fakeSessionResolver{}, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/thoughts", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if location := rr.Header().Get("Location"); location != "/login" {
		t.Fatalf("Location = %q, want %q", location, "/login")
	}
}

func TestUnauthorizedAPIThoughtListReturns401(t *testing.T) {
	router := NewWebRouter(config.Config{CookieSecure: false, CSRFKey: "test-csrf-key"}, fakeSessionResolver{}, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/thoughts", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestThoughtCreateRouteRequiresCSRF(t *testing.T) {
	handler := &fakeWebThoughtHandlers{}
	token := "session-token"
	router := NewWebRouter(config.Config{CookieSecure: false, CSRFKey: "test-csrf-key"}, fakeValidSessionResolver{user: auth.User{ID: uuid.New(), Username: "alice"}}, nil, nil, handler, nil)
	req := httptest.NewRequest(http.MethodPost, "/thoughts", strings.NewReader("content=remember"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if handler.createCalled {
		t.Fatal("create handler called without valid CSRF")
	}
}

func TestAuthenticatedAPIThoughtListUsesHandler(t *testing.T) {
	handler := &fakeAPIThoughtHandlers{}
	token := "session-token"
	router := NewWebRouter(config.Config{CookieSecure: false, CSRFKey: "test-csrf-key"}, fakeValidSessionResolver{user: auth.User{ID: uuid.New(), Username: "alice"}}, nil, nil, nil, handler)
	req := httptest.NewRequest(http.MethodGet, "/api/thoughts", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !handler.listCalled {
		t.Fatal("list handler was not called")
	}
}

func TestAuthenticatedAPIRelatedThoughtsUsesHandler(t *testing.T) {
	handler := &fakeAPIThoughtHandlers{}
	token := "session-token"
	router := NewWebRouter(config.Config{CookieSecure: false, CSRFKey: "test-csrf-key"}, fakeValidSessionResolver{user: auth.User{ID: uuid.New(), Username: "alice"}}, nil, nil, nil, handler)
	req := httptest.NewRequest(http.MethodGet, "/api/thoughts/"+uuid.New().String()+"/related", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if !handler.relatedCalled {
		t.Fatal("related handler was not called")
	}
}

func TestAuthenticatedAPIThoughtCreateUsesHandlerWithCSRF(t *testing.T) {
	handler := &fakeAPIThoughtHandlers{}
	token := "session-token"
	router := NewWebRouter(config.Config{CookieSecure: false, CSRFKey: "test-csrf-key"}, fakeValidSessionResolver{user: auth.User{ID: uuid.New(), Username: "alice"}}, nil, nil, nil, handler)
	req := httptest.NewRequest(http.MethodPost, "/api/thoughts", strings.NewReader(`{"content":"remember"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", security.Token("test-csrf-key", token))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if !handler.createCalled {
		t.Fatal("create handler was not called")
	}
}

type fakeSessionResolver struct{}

func (fakeSessionResolver) AuthenticatePassword(ctx context.Context, username, password string) (auth.Session, error) {
	return auth.Session{}, nil
}

func (fakeSessionResolver) LogoutSession(ctx context.Context, token string) error {
	return nil
}

func (fakeSessionResolver) RequireSession(ctx context.Context, token string) (auth.User, auth.Session, error) {
	return auth.User{}, auth.Session{}, auth.ErrInvalidSession
}

func (fakeSessionResolver) RequireMCPTokenUser(ctx context.Context, token string) (auth.User, error) {
	return auth.User{}, nil
}

func (fakeSessionResolver) SessionTTL() time.Duration {
	return 24 * time.Hour
}

type fakeValidSessionResolver struct {
	user auth.User
}

func (f fakeValidSessionResolver) AuthenticatePassword(ctx context.Context, username, password string) (auth.Session, error) {
	return auth.Session{}, nil
}

func (f fakeValidSessionResolver) LogoutSession(ctx context.Context, token string) error {
	return nil
}

func (f fakeValidSessionResolver) RequireSession(ctx context.Context, token string) (auth.User, auth.Session, error) {
	return f.user, auth.Session{Token: token}, nil
}

func (f fakeValidSessionResolver) RequireMCPTokenUser(ctx context.Context, token string) (auth.User, error) {
	return auth.User{}, nil
}

func (f fakeValidSessionResolver) SessionTTL() time.Duration {
	return 24 * time.Hour
}

type fakeWebThoughtHandlers struct {
	createCalled bool
}

func (f *fakeWebThoughtHandlers) Index(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (f *fakeWebThoughtHandlers) Thoughts(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (f *fakeWebThoughtHandlers) ThoughtDetail(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (f *fakeWebThoughtHandlers) DeleteConfirm(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (f *fakeWebThoughtHandlers) Update(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (f *fakeWebThoughtHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusSeeOther)
}
func (f *fakeWebThoughtHandlers) Retry(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusSeeOther)
}
func (f *fakeWebThoughtHandlers) Create(w http.ResponseWriter, r *http.Request) {
	f.createCalled = true
	w.WriteHeader(http.StatusSeeOther)
}

type fakeAPIThoughtHandlers struct {
	listCalled    bool
	createCalled  bool
	relatedCalled bool
}

func (f *fakeAPIThoughtHandlers) Create(w http.ResponseWriter, r *http.Request) {
	f.createCalled = true
	w.WriteHeader(http.StatusCreated)
}
func (f *fakeAPIThoughtHandlers) List(w http.ResponseWriter, r *http.Request) {
	f.listCalled = true
	w.WriteHeader(http.StatusOK)
}
func (f *fakeAPIThoughtHandlers) Get(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (f *fakeAPIThoughtHandlers) Related(w http.ResponseWriter, r *http.Request) {
	f.relatedCalled = true
	w.WriteHeader(http.StatusOK)
}
func (f *fakeAPIThoughtHandlers) Update(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (f *fakeAPIThoughtHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
func (f *fakeAPIThoughtHandlers) Retry(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	return uuid.New()
}

var _ = api.NewThoughtHandlers
var _ = web.NewHandlers
