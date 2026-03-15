package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

func TestUpdateUserCreatesDefaultTokenWhenUserHasNoTokens(t *testing.T) {
	repo := &fakeRepository{
		upsertedUser: auth.User{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Username: "demo"},
	}
	service := NewService(repo)
	service.hashPassword = func(password string) (string, error) { return "hashed-" + password, nil }
	service.newToken = func() (string, error) { return "plain-default-token", nil }
	service.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	result, err := service.UpdateUser(context.Background(), "demo", "secret", "default")
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if repo.upsertPasswordHash != "hashed-secret" {
		t.Fatalf("password hash = %q, want hashed-secret", repo.upsertPasswordHash)
	}
	if result.CreatedToken == nil {
		t.Fatal("expected default token to be created")
	}
	if result.CreatedToken.Label != "default" || result.CreatedToken.Token != "plain-default-token" {
		t.Fatalf("created token = %+v, want label default with plaintext token", *result.CreatedToken)
	}
	if len(repo.createdTokens) != 1 || repo.createdTokens[0].TokenHash != auth.HashToken("plain-default-token") {
		t.Fatalf("created tokens = %+v, want hashed default token", repo.createdTokens)
	}
}

func TestUpdateUserDoesNotRotateExistingTokens(t *testing.T) {
	repo := &fakeRepository{
		upsertedUser: auth.User{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Username: "demo"},
		listedTokens: []auth.MCPToken{{ID: uuid.New(), UserID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Label: "default"}},
	}
	service := NewService(repo)
	service.hashPassword = func(password string) (string, error) { return "hashed-" + password, nil }
	service.newToken = func() (string, error) { return "should-not-be-used", nil }

	result, err := service.UpdateUser(context.Background(), "demo", "secret", "default")
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}
	if result.CreatedToken != nil {
		t.Fatalf("CreatedToken = %+v, want nil", result.CreatedToken)
	}
	if len(repo.createdTokens) != 0 {
		t.Fatalf("created tokens = %+v, want none", repo.createdTokens)
	}
}

func TestCreateTokenReplacesLabelAndReturnsPlaintext(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &fakeRepository{
		foundUser: auth.User{ID: userID, Username: "demo"},
	}
	service := NewService(repo)
	service.newToken = func() (string, error) { return "plain-rotated-token", nil }
	service.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	created, err := service.CreateToken(context.Background(), "demo", "default")
	if err != nil {
		t.Fatalf("CreateToken() error = %v", err)
	}
	if created.Token != "plain-rotated-token" || created.Label != "default" {
		t.Fatalf("created token = %+v, want plaintext rotated token", created)
	}
	if len(repo.deletedLabels) != 1 || repo.deletedLabels[0] != "default" {
		t.Fatalf("deleted labels = %+v, want [default]", repo.deletedLabels)
	}
	if len(repo.createdTokens) != 1 || repo.createdTokens[0].TokenHash != auth.HashToken("plain-rotated-token") {
		t.Fatalf("created tokens = %+v, want hashed rotated token", repo.createdTokens)
	}
}

func TestListTokensFindsUserByUsername(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &fakeRepository{
		foundUser:    auth.User{ID: userID, Username: "demo"},
		listedTokens: []auth.MCPToken{{ID: uuid.New(), UserID: userID, Label: "default"}},
	}
	service := NewService(repo)

	tokens, err := service.ListTokens(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListTokens() error = %v", err)
	}
	if len(tokens) != 1 || tokens[0].Label != "default" {
		t.Fatalf("tokens = %+v, want listed default token", tokens)
	}
}

func TestDeleteTokenDeletesByLabel(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	repo := &fakeRepository{foundUser: auth.User{ID: userID, Username: "demo"}, deleteTokenCount: 1}
	service := NewService(repo)

	deletedCount, err := service.DeleteToken(context.Background(), "demo", "default")
	if err != nil {
		t.Fatalf("DeleteToken() error = %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("deleted count = %d, want 1", deletedCount)
	}
}

func TestDeleteUserDeletesByUsername(t *testing.T) {
	repo := &fakeRepository{deleteUserCount: 1}
	service := NewService(repo)

	if err := service.DeleteUser(context.Background(), "demo"); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	if repo.deletedUsernames[0] != "demo" {
		t.Fatalf("deleted usernames = %+v, want demo", repo.deletedUsernames)
	}
}

type fakeRepository struct {
	foundUser          auth.User
	upsertedUser       auth.User
	listedTokens       []auth.MCPToken
	createdTokens      []auth.MCPToken
	deletedLabels      []string
	deletedUsernames   []string
	upsertPasswordHash string
	deleteTokenCount   int64
	deleteUserCount    int64
}

func (f *fakeRepository) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	if f.foundUser.ID == uuid.Nil && f.upsertedUser.ID == uuid.Nil {
		return auth.User{}, auth.ErrUserNotFound
	}
	if f.foundUser.ID != uuid.Nil {
		return f.foundUser, nil
	}
	return f.upsertedUser, nil
}

func (f *fakeRepository) UpsertUser(ctx context.Context, username, passwordHash string, now time.Time) (auth.User, error) {
	f.upsertPasswordHash = passwordHash
	if f.upsertedUser.ID == uuid.Nil {
		f.upsertedUser = auth.User{ID: uuid.New(), Username: username}
	}
	return f.upsertedUser, nil
}

func (f *fakeRepository) DeleteUserByUsername(ctx context.Context, username string) (int64, error) {
	f.deletedUsernames = append(f.deletedUsernames, username)
	return f.deleteUserCount, nil
}

func (f *fakeRepository) ListTokensByUserID(ctx context.Context, userID uuid.UUID) ([]auth.MCPToken, error) {
	return append([]auth.MCPToken(nil), f.listedTokens...), nil
}

func (f *fakeRepository) DeleteTokensByUserIDAndLabel(ctx context.Context, userID uuid.UUID, label string) (int64, error) {
	f.deletedLabels = append(f.deletedLabels, label)
	return f.deleteTokenCount, nil
}

func (f *fakeRepository) CreateToken(ctx context.Context, token auth.MCPToken) error {
	if token.ID == uuid.Nil {
		return errors.New("missing token id")
	}
	f.createdTokens = append(f.createdTokens, token)
	return nil
}
