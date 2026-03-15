package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const defaultTouchInterval = 5 * time.Minute

type Service struct {
	repo          Repository
	sessionTTL    time.Duration
	touchInterval time.Duration
	now           func() time.Time
	newToken      func() (string, error)
}

func NewService(repo Repository, sessionTTL time.Duration) *Service {
	return &Service{
		repo:          repo,
		sessionTTL:    sessionTTL,
		touchInterval: defaultTouchInterval,
		now:           func() time.Time { return time.Now().UTC() },
		newToken:      NewToken,
	}
}

func (s *Service) AuthenticatePassword(ctx context.Context, username, password string) (Session, error) {
	user, err := s.repo.FindUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, err
	}
	if user.DisabledAt != nil {
		return Session{}, ErrUserDisabled
	}
	if err := CheckPassword(user.PasswordHash, password); err != nil {
		return Session{}, ErrInvalidCredentials
	}

	token, err := s.newToken()
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	session, err := s.repo.CreateSession(ctx, CreateSessionParams{
		ID:               uuid.New(),
		UserID:           user.ID,
		SessionTokenHash: HashToken(token),
		ExpiresAt:        now.Add(s.sessionTTL),
		CreatedAt:        now,
		LastSeenAt:       now,
	})
	if err != nil {
		return Session{}, err
	}
	session.Token = token
	return session, nil
}

func (s *Service) LogoutSession(ctx context.Context, token string) error {
	hashed := HashToken(token)
	session, _, err := s.repo.FindSessionByTokenHash(ctx, hashed)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidSession
		}
		return err
	}
	return s.repo.DeleteSession(ctx, session.ID)
}

func (s *Service) RequireSession(ctx context.Context, token string) (User, Session, error) {
	hashed := HashToken(token)
	session, user, err := s.repo.FindSessionByTokenHash(ctx, hashed)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return User{}, Session{}, ErrInvalidSession
		}
		return User{}, Session{}, err
	}
	if user.DisabledAt != nil {
		return User{}, Session{}, ErrUserDisabled
	}
	if s.now().After(session.ExpiresAt) {
		return User{}, Session{}, ErrSessionExpired
	}
	if s.shouldTouchSession(session.LastSeenAt) {
		_ = s.repo.TouchSessionActivity(ctx, session.ID, s.now())
	}
	return user, session, nil
}

func (s *Service) RequireMCPTokenUser(ctx context.Context, token string) (User, error) {
	hashed := HashToken(token)
	user, mcpToken, err := s.repo.FindUserByMCPTokenHash(ctx, hashed)
	if err != nil {
		return User{}, err
	}
	if user.DisabledAt != nil {
		return User{}, ErrUserDisabled
	}
	if mcpToken.RevokedAt != nil {
		return User{}, ErrTokenRevoked
	}
	if s.shouldTouchToken(mcpToken.LastUsedAt) {
		_ = s.repo.TouchMCPTokenActivity(ctx, mcpToken.ID, s.now())
	}
	return user, nil
}

func (s *Service) shouldTouchSession(lastSeenAt time.Time) bool {
	if lastSeenAt.IsZero() {
		return true
	}
	return s.now().Sub(lastSeenAt) >= s.touchInterval
}

func (s *Service) shouldTouchToken(lastUsedAt time.Time) bool {
	if lastUsedAt.IsZero() {
		return true
	}
	return s.now().Sub(lastUsedAt) >= s.touchInterval
}
