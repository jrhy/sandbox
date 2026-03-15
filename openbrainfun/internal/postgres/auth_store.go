package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type commandTag interface {
	RowsAffected() int64
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
	ExecContext(ctx context.Context, query string, args ...any) (commandTag, error)
}

type pgxQueryer interface {
	QueryRow(ctx context.Context, query string, args ...any) pgx.Row
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
}

type pgxAuthAdapter struct {
	db pgxQueryer
}

func (a pgxAuthAdapter) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return a.db.QueryRow(ctx, query, args...)
}

func (a pgxAuthAdapter) ExecContext(ctx context.Context, query string, args ...any) (commandTag, error) {
	return a.db.Exec(ctx, query, args...)
}

type AuthStore struct {
	db queryer
}

func NewAuthStore(db queryer) *AuthStore {
	return &AuthStore{db: db}
}

func NewAuthStoreFromPGX(db pgxQueryer) *AuthStore {
	return &AuthStore{db: pgxAuthAdapter{db: db}}
}

func (s *AuthStore) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	const query = `
select id, username, password_hash, created_at, updated_at, disabled_at
from users
where username = $1
`

	var user auth.User
	var disabledAt sql.NullTime
	err := s.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&disabledAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, err
	}
	if disabledAt.Valid {
		user.DisabledAt = &disabledAt.Time
	}
	return user, nil
}

func (s *AuthStore) CreateSession(ctx context.Context, params auth.CreateSessionParams) (auth.Session, error) {
	const query = `
insert into web_sessions (id, user_id, session_token_hash, expires_at, created_at, last_seen_at)
values ($1, $2, $3, $4, $5, $6)
`
	if _, err := s.db.ExecContext(ctx, query,
		params.ID,
		params.UserID,
		params.SessionTokenHash,
		params.ExpiresAt,
		params.CreatedAt,
		params.LastSeenAt,
	); err != nil {
		return auth.Session{}, err
	}
	return auth.Session{
		ID:               params.ID,
		UserID:           params.UserID,
		SessionTokenHash: params.SessionTokenHash,
		ExpiresAt:        params.ExpiresAt,
		CreatedAt:        params.CreatedAt,
		LastSeenAt:       params.LastSeenAt,
	}, nil
}

func (s *AuthStore) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	const query = `delete from web_sessions where id = $1`
	_, err := s.db.ExecContext(ctx, query, sessionID)
	return err
}

func (s *AuthStore) FindSessionByTokenHash(ctx context.Context, tokenHash string) (auth.Session, auth.User, error) {
	const query = `
select ws.id, ws.user_id, ws.session_token_hash, ws.expires_at, ws.created_at, ws.last_seen_at,
       u.id, u.username, u.password_hash, u.created_at, u.updated_at, u.disabled_at
from web_sessions ws
join users u on u.id = ws.user_id
where ws.session_token_hash = $1
`
	var session auth.Session
	var user auth.User
	var disabledAt sql.NullTime
	err := s.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&session.ID,
		&session.UserID,
		&session.SessionTokenHash,
		&session.ExpiresAt,
		&session.CreatedAt,
		&session.LastSeenAt,
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&disabledAt,
	)
	if err != nil {
		return auth.Session{}, auth.User{}, err
	}
	if disabledAt.Valid {
		user.DisabledAt = &disabledAt.Time
	}
	return session, user, nil
}

func (s *AuthStore) FindUserByMCPTokenHash(ctx context.Context, tokenHash string) (auth.User, auth.MCPToken, error) {
	const query = `
select u.id, u.username, u.password_hash, u.created_at, u.updated_at, u.disabled_at,
       mt.id, mt.user_id, mt.token_hash, mt.label, mt.created_at, mt.last_used_at, mt.revoked_at
from mcp_tokens mt
join users u on u.id = mt.user_id
where mt.token_hash = $1
`
	var user auth.User
	var token auth.MCPToken
	var disabledAt sql.NullTime
	var lastUsedAt sql.NullTime
	var revokedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&disabledAt,
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.Label,
		&token.CreatedAt,
		&lastUsedAt,
		&revokedAt,
	)
	if err != nil {
		return auth.User{}, auth.MCPToken{}, err
	}
	if disabledAt.Valid {
		user.DisabledAt = &disabledAt.Time
	}
	if lastUsedAt.Valid {
		token.LastUsedAt = lastUsedAt.Time
	}
	if revokedAt.Valid {
		token.RevokedAt = &revokedAt.Time
	}
	return user, token, nil
}

func (s *AuthStore) TouchSessionActivity(ctx context.Context, sessionID uuid.UUID, seenAt time.Time) error {
	const query = `update web_sessions set last_seen_at = $2 where id = $1`
	_, err := s.db.ExecContext(ctx, query, sessionID, seenAt)
	return err
}

func (s *AuthStore) TouchMCPTokenActivity(ctx context.Context, tokenID uuid.UUID, usedAt time.Time) error {
	const query = `update mcp_tokens set last_used_at = $2 where id = $1`
	_, err := s.db.ExecContext(ctx, query, tokenID, usedAt)
	return err
}
