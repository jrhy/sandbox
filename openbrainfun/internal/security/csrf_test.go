package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

func TestCookieAuthenticatedWriteRejectsMissingCSRF(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/thoughts", strings.NewReader(`{"content":"hello"}`))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "plain-session-token"})
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, auth.User{ID: uuid.New()}))
	rr := httptest.NewRecorder()

	NewCSRFMiddleware("test-csrf-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}
