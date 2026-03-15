package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

func TestPostSessionSetsCookie(t *testing.T) {
	handlers := NewSessionHandlers(fakeAuthService{}, false, "test-csrf-key")
	body := bytes.NewBufferString(`{"username":"alice","password":"secret-pass"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/session", body)
	rr := httptest.NewRecorder()

	handlers.PostSession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	cookie := rr.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("expected session cookie to be set")
	}
	if !cookie[0].HttpOnly {
		t.Fatal("expected HttpOnly session cookie")
	}
	if cookie[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite = %v, want %v", cookie[0].SameSite, http.SameSiteLaxMode)
	}
	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["csrf_token"] == "" {
		t.Fatalf("csrf_token missing from response: %s", rr.Body.String())
	}
}

type fakeAuthService struct{}

func (fakeAuthService) AuthenticatePassword(ctx context.Context, username, password string) (auth.Session, error) {
	now := time.Unix(1700000000, 0).UTC()
	return auth.Session{
		ID:               uuid.New(),
		UserID:           uuid.New(),
		Token:            "plain-session-token",
		SessionTokenHash: auth.HashToken("plain-session-token"),
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
		LastSeenAt:       now,
	}, nil
}

func (fakeAuthService) LogoutSession(ctx context.Context, token string) error {
	return nil
}
