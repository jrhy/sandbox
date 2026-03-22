package app

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
	"github.com/jrhy/sandbox/openbrainfun/internal/config"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestBuildComposesAuthenticatedThoughtCreation(t *testing.T) {
	passwordHash, err := auth.HashPassword("secret-pass")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	userID := uuid.New()
	authRepo := &fakeAuthRepo{
		user: auth.User{ID: userID, Username: "alice", PasswordHash: passwordHash},
	}
	thoughtRepo := &fakeThoughtRepo{}
	cfg := config.Config{
		CookieSecure: false,
		CSRFKey:      "test-csrf-key",
		SessionTTL:   24 * time.Hour,
	}

	runtime := Build(cfg, authRepo, thoughtRepo, embed.NewFake(nil), metadata.NewFake(nil))

	loginReq := httptest.NewRequest(http.MethodPost, "/api/session", bytes.NewBufferString(`{"username":"alice","password":"secret-pass"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	runtime.WebHandler.ServeHTTP(loginResp, loginReq)

	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.Code)
	}
	cookie := loginResp.Result().Cookies()[0]
	var sessionBody struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(loginResp.Body.Bytes(), &sessionBody); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if sessionBody.CSRFToken == "" {
		t.Fatal("csrf_token = empty, want token")
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/thoughts", bytes.NewBufferString(`{"content":"Remember MCP auth","exposure_scope":"remote_ok","user_tags":["mcp","auth"]}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", sessionBody.CSRFToken)
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	runtime.WebHandler.ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", createResp.Code)
	}
	if thoughtRepo.created.UserID != userID {
		t.Fatalf("created.UserID = %s, want %s", thoughtRepo.created.UserID, userID)
	}
	if thoughtRepo.created.Content != "Remember MCP auth" {
		t.Fatalf("created.Content = %q, want request content", thoughtRepo.created.Content)
	}
}

func TestBuildComposesMCPHandler(t *testing.T) {
	cfg := config.Config{
		CookieSecure: false,
		CSRFKey:      "test-csrf-key",
		SessionTTL:   24 * time.Hour,
	}

	runtime := Build(cfg, &fakeAuthRepo{}, &fakeThoughtRepo{}, embed.NewFake(nil), metadata.NewFake(nil))
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	runtime.MCPHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

type fakeAuthRepo struct {
	user    auth.User
	session auth.Session
}

func (f *fakeAuthRepo) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	if username != f.user.Username {
		return auth.User{}, auth.ErrUserNotFound
	}
	return f.user, nil
}

func (f *fakeAuthRepo) CreateSession(ctx context.Context, params auth.CreateSessionParams) (auth.Session, error) {
	f.session = auth.Session{
		ID:               params.ID,
		UserID:           params.UserID,
		SessionTokenHash: params.SessionTokenHash,
		ExpiresAt:        params.ExpiresAt,
		CreatedAt:        params.CreatedAt,
		LastSeenAt:       params.LastSeenAt,
	}
	return f.session, nil
}

func (f *fakeAuthRepo) DeleteSession(ctx context.Context, sessionID uuid.UUID) error { return nil }

func (f *fakeAuthRepo) FindSessionByTokenHash(ctx context.Context, tokenHash string) (auth.Session, auth.User, error) {
	if f.session.SessionTokenHash != tokenHash {
		return auth.Session{}, auth.User{}, auth.ErrUserNotFound
	}
	return f.session, f.user, nil
}

func (f *fakeAuthRepo) FindUserByMCPTokenHash(ctx context.Context, tokenHash string) (auth.User, auth.MCPToken, error) {
	return auth.User{}, auth.MCPToken{}, auth.ErrUserNotFound
}

func (f *fakeAuthRepo) TouchSessionActivity(ctx context.Context, sessionID uuid.UUID, seenAt time.Time) error {
	return nil
}

func (f *fakeAuthRepo) TouchMCPTokenActivity(ctx context.Context, tokenID uuid.UUID, usedAt time.Time) error {
	return nil
}

type fakeThoughtRepo struct {
	created thoughts.Thought
}

func (f *fakeThoughtRepo) CreateThought(ctx context.Context, thought thoughts.Thought) (thoughts.Thought, error) {
	f.created = thought
	return thought, nil
}

func (f *fakeThoughtRepo) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	return thoughts.Thought{}, thoughts.ErrThoughtNotFound
}

func (f *fakeThoughtRepo) UpdateThought(ctx context.Context, params thoughts.UpdateThoughtParams) (thoughts.Thought, error) {
	return thoughts.Thought{}, thoughts.ErrThoughtNotFound
}

func (f *fakeThoughtRepo) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	return thoughts.ErrThoughtNotFound
}

func (f *fakeThoughtRepo) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	return nil, nil
}

func (f *fakeThoughtRepo) SearchKeyword(ctx context.Context, params thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	return nil, nil
}

func (f *fakeThoughtRepo) SearchSemantic(ctx context.Context, params thoughts.SearchSemanticParams) ([]thoughts.ScoredThought, error) {
	return nil, nil
}

func (f *fakeThoughtRepo) RelatedThoughts(ctx context.Context, params thoughts.RelatedThoughtsParams) ([]thoughts.ScoredThought, error) {
	return nil, nil
}

func (f *fakeThoughtRepo) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	return thoughts.Thought{}, thoughts.ErrThoughtNotFound
}

func (f *fakeThoughtRepo) ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error) {
	return nil, nil
}

func (f *fakeThoughtRepo) MarkProcessed(ctx context.Context, params thoughts.MarkProcessedParams) error {
	return nil
}
func (f *fakeThoughtRepo) MarkEmbeddingFailed(ctx context.Context, params thoughts.MarkEmbeddingFailedParams) error {
	return nil
}
func (f *fakeThoughtRepo) ReconcileModels(ctx context.Context, params thoughts.ReconcileModelsParams) (thoughts.ReconcileModelsResult, error) {
	return thoughts.ReconcileModelsResult{}, nil
}
