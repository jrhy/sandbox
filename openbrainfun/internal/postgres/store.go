package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"openbrainfun/internal/thoughts"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func Connect(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	return NewStore(pool), nil
}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) CreateThought(ctx context.Context, params thoughts.CreateThoughtParams) (thoughts.Thought, error) {
	t := params.Thought
	_, err := s.pool.Exec(ctx, `INSERT INTO thoughts (id, content, exposure_scope, user_tags, source, ingest_status, failure_reason, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, t.ID, t.Content, t.ExposureScope, t.UserTags, t.Source, t.IngestStatus, t.FailureReason, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return thoughts.Thought{}, err
	}
	return t, nil
}

func (s *Store) GetThought(ctx context.Context, id uuid.UUID) (thoughts.Thought, error) {
	rows, err := s.ListRecent(ctx, thoughts.ListRecentParams{Limit: 100})
	if err != nil {
		return thoughts.Thought{}, err
	}
	for _, t := range rows {
		if t.ID == id {
			return t, nil
		}
	}
	return thoughts.Thought{}, fmt.Errorf("not found")
}

func (s *Store) ListRecent(ctx context.Context, params thoughts.ListRecentParams) ([]thoughts.Thought, error) {
	if params.Limit <= 0 {
		params.Limit = 20
	}
	rows, err := s.pool.Query(ctx, `SELECT id, content, exposure_scope, user_tags, source, ingest_status, failure_reason, created_at, updated_at FROM thoughts ORDER BY created_at DESC LIMIT $1`, params.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []thoughts.Thought
	for rows.Next() {
		var t thoughts.Thought
		if err := rows.Scan(&t.ID, &t.Content, &t.ExposureScope, &t.UserTags, &t.Source, &t.IngestStatus, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SearchKeyword(ctx context.Context, params thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	if params.Limit <= 0 {
		params.Limit = 20
	}
	rows, err := s.pool.Query(ctx, `SELECT id, content, exposure_scope, user_tags, source, ingest_status, failure_reason, created_at, updated_at
FROM thoughts WHERE content ILIKE '%' || $1 || '%' ORDER BY created_at DESC LIMIT $2`, params.Query, params.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []thoughts.Thought
	for rows.Next() {
		var t thoughts.Thought
		if err := rows.Scan(&t.ID, &t.Content, &t.ExposureScope, &t.UserTags, &t.Source, &t.IngestStatus, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SearchSemantic(ctx context.Context, params thoughts.SearchSemanticParams) ([]thoughts.Thought, error) {
	// Minimal v1 fallback: return recent rows. Vector search wiring can be tightened later.
	return s.ListRecent(ctx, thoughts.ListRecentParams{Limit: params.Limit})
}

func (s *Store) ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `SELECT id, content, exposure_scope, user_tags, source, ingest_status, failure_reason, created_at, updated_at
FROM thoughts WHERE ingest_status='pending' ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []thoughts.Thought
	for rows.Next() {
		var t thoughts.Thought
		if err := rows.Scan(&t.ID, &t.Content, &t.ExposureScope, &t.UserTags, &t.Source, &t.IngestStatus, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) MarkReady(ctx context.Context, params thoughts.MarkReadyParams) error {
	_, err := s.pool.Exec(ctx, `UPDATE thoughts SET ingest_status='ready', failure_reason='', updated_at=now() WHERE id=$1`, params.ID)
	return err
}

func (s *Store) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	_, err := s.pool.Exec(ctx, `UPDATE thoughts SET ingest_status='failed', failure_reason=$2, updated_at=now() WHERE id=$1`, id, reason)
	return err
}
