package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidUsername = errors.New("invalid username")

type User struct {
	ID           uuid.UUID
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DisabledAt   *time.Time
}

type Session struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	Token            string
	SessionTokenHash string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	LastSeenAt       time.Time
}

type MCPToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	Label      string
	CreatedAt  time.Time
	LastUsedAt time.Time
	RevokedAt  *time.Time
}

func NewUser(id uuid.UUID, username, passwordHash string) (User, error) {
	trimmedUsername := strings.TrimSpace(username)
	if trimmedUsername == "" {
		return User{}, ErrInvalidUsername
	}

	return User{
		ID:           id,
		Username:     trimmedUsername,
		PasswordHash: passwordHash,
	}, nil
}
