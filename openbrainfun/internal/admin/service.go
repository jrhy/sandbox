package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

const defaultTokenLabel = "default"

type Repository interface {
	FindUserByUsername(ctx context.Context, username string) (auth.User, error)
	UpsertUser(ctx context.Context, username, passwordHash string, now time.Time) (auth.User, error)
	DeleteUserByUsername(ctx context.Context, username string) (int64, error)
	ListTokensByUserID(ctx context.Context, userID uuid.UUID) ([]auth.MCPToken, error)
	DeleteTokensByUserIDAndLabel(ctx context.Context, userID uuid.UUID, label string) (int64, error)
	CreateToken(ctx context.Context, token auth.MCPToken) error
}

type Service struct {
	repo         Repository
	now          func() time.Time
	hashPassword func(password string) (string, error)
	newToken     func() (string, error)
}

type UpdateUserResult struct {
	User         auth.User
	CreatedToken *CreatedToken
}

type CreatedToken struct {
	Label string
	Token string
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:         repo,
		now:          func() time.Time { return time.Now().UTC() },
		hashPassword: auth.HashPassword,
		newToken:     auth.NewToken,
	}
}

func (s *Service) UpdateUser(ctx context.Context, username, password, tokenLabel string) (UpdateUserResult, error) {
	if tokenLabel == "" {
		tokenLabel = defaultTokenLabel
	}

	passwordHash, err := s.hashPassword(password)
	if err != nil {
		return UpdateUserResult{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.repo.UpsertUser(ctx, username, passwordHash, s.now())
	if err != nil {
		return UpdateUserResult{}, fmt.Errorf("upsert user: %w", err)
	}

	tokens, err := s.repo.ListTokensByUserID(ctx, user.ID)
	if err != nil {
		return UpdateUserResult{}, fmt.Errorf("list tokens: %w", err)
	}

	result := UpdateUserResult{User: user}
	if len(tokens) == 0 {
		createdToken, err := s.createTokenForUser(ctx, user, tokenLabel)
		if err != nil {
			return UpdateUserResult{}, err
		}
		result.CreatedToken = &createdToken
	}

	return result, nil
}

func (s *Service) DeleteUser(ctx context.Context, username string) error {
	deletedCount, err := s.repo.DeleteUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if deletedCount == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

func (s *Service) CreateToken(ctx context.Context, username, label string) (CreatedToken, error) {
	if label == "" {
		label = defaultTokenLabel
	}

	user, err := s.repo.FindUserByUsername(ctx, username)
	if err != nil {
		return CreatedToken{}, err
	}
	return s.createTokenForUser(ctx, user, label)
}

func (s *Service) ListTokens(ctx context.Context, username string) ([]auth.MCPToken, error) {
	user, err := s.repo.FindUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	return s.repo.ListTokensByUserID(ctx, user.ID)
}

func (s *Service) DeleteToken(ctx context.Context, username, label string) (int64, error) {
	user, err := s.repo.FindUserByUsername(ctx, username)
	if err != nil {
		return 0, err
	}

	return s.repo.DeleteTokensByUserIDAndLabel(ctx, user.ID, label)
}

func (s *Service) createTokenForUser(ctx context.Context, user auth.User, label string) (CreatedToken, error) {
	plaintextToken, err := s.newToken()
	if err != nil {
		return CreatedToken{}, fmt.Errorf("generate token: %w", err)
	}

	if _, err := s.repo.DeleteTokensByUserIDAndLabel(ctx, user.ID, label); err != nil {
		return CreatedToken{}, fmt.Errorf("delete existing token label %q: %w", label, err)
	}

	now := s.now()
	token := auth.MCPToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: auth.HashToken(plaintextToken),
		Label:     label,
		CreatedAt: now,
	}
	if err := s.repo.CreateToken(ctx, token); err != nil {
		return CreatedToken{}, fmt.Errorf("create token: %w", err)
	}

	return CreatedToken{
		Label: label,
		Token: plaintextToken,
	}, nil
}
