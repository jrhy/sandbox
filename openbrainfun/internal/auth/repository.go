package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidSession     = errors.New("invalid session")
	ErrSessionExpired     = errors.New("session expired")
	ErrTokenRevoked       = errors.New("token revoked")
	ErrUserDisabled       = errors.New("user disabled")
	ErrUserNotFound       = errors.New("user not found")
)

type CreateSessionParams struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	SessionTokenHash string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	LastSeenAt       time.Time
}

type Repository interface {
	FindUserByUsername(ctx context.Context, username string) (User, error)
	CreateSession(ctx context.Context, params CreateSessionParams) (Session, error)
	DeleteSession(ctx context.Context, sessionID uuid.UUID) error
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (Session, User, error)
	FindUserByMCPTokenHash(ctx context.Context, tokenHash string) (User, MCPToken, error)
	TouchSessionActivity(ctx context.Context, sessionID uuid.UUID, seenAt time.Time) error
	TouchMCPTokenActivity(ctx context.Context, tokenID uuid.UUID, usedAt time.Time) error
}
