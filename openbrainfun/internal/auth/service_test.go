package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAuthenticateCreatesSessionForValidCredentials(t *testing.T) {
	repo := &fakeRepo{user: User{ID: uuid.New(), Username: "alice", PasswordHash: mustHash(t, "secret-pass")}}
	svc := NewService(repo, 24*time.Hour)
	svc.newToken = func() (string, error) { return "plain-session-token", nil }
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	session, err := svc.AuthenticatePassword(context.Background(), "alice", "secret-pass")
	if err != nil {
		t.Fatalf("AuthenticatePassword() error = %v", err)
	}
	if session.UserID != repo.user.ID {
		t.Fatalf("UserID = %s, want %s", session.UserID, repo.user.ID)
	}
	if session.Token != "plain-session-token" {
		t.Fatalf("Token = %q, want %q", session.Token, "plain-session-token")
	}
	if session.SessionTokenHash != HashToken("plain-session-token") {
		t.Fatalf("SessionTokenHash = %q, want hashed token", session.SessionTokenHash)
	}
}

func TestRequireMCPTokenUserRejectsRevokedToken(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	repo := &fakeRepo{
		mcpUser:  User{ID: uuid.New(), Username: "alice"},
		mcpToken: MCPToken{ID: uuid.New(), UserID: uuid.New(), RevokedAt: &now},
	}
	svc := NewService(repo, 24*time.Hour)

	_, err := svc.RequireMCPTokenUser(context.Background(), "revoked-token")
	if !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("error = %v, want ErrTokenRevoked", err)
	}
}

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	return hash
}

type fakeRepo struct {
	user             User
	session          Session
	mcpUser          User
	mcpToken         MCPToken
	findUserErr      error
	createSessionErr error
	findSessionErr   error
	findMCPTokenErr  error
}

func (f *fakeRepo) FindUserByUsername(ctx context.Context, username string) (User, error) {
	return f.user, f.findUserErr
}

func (f *fakeRepo) CreateSession(ctx context.Context, params CreateSessionParams) (Session, error) {
	if f.createSessionErr != nil {
		return Session{}, f.createSessionErr
	}
	return Session{
		ID:               params.ID,
		UserID:           params.UserID,
		SessionTokenHash: params.SessionTokenHash,
		ExpiresAt:        params.ExpiresAt,
		CreatedAt:        params.CreatedAt,
		LastSeenAt:       params.LastSeenAt,
	}, nil
}

func (f *fakeRepo) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	return nil
}

func (f *fakeRepo) FindSessionByTokenHash(ctx context.Context, tokenHash string) (Session, User, error) {
	return f.session, f.user, f.findSessionErr
}

func (f *fakeRepo) FindUserByMCPTokenHash(ctx context.Context, tokenHash string) (User, MCPToken, error) {
	return f.mcpUser, f.mcpToken, f.findMCPTokenErr
}

func (f *fakeRepo) TouchSessionActivity(ctx context.Context, sessionID uuid.UUID, seenAt time.Time) error {
	return nil
}

func (f *fakeRepo) TouchMCPTokenActivity(ctx context.Context, tokenID uuid.UUID, usedAt time.Time) error {
	return nil
}
