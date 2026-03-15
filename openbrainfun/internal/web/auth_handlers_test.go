package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginPageShowsForm(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rr := httptest.NewRecorder()

	NewAuthHandlers(nil, false, "test-csrf-key").LoginPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="username"`) || !strings.Contains(body, `>Log in<`) {
		t.Fatalf("body missing login form: %s", body)
	}
}
