package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

type thoughtQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type ThoughtStore struct {
	db thoughtQueryer
}

func NewThoughtStore(db thoughtQueryer) *ThoughtStore {
	return &ThoughtStore{db: db}
}

func (s *ThoughtStore) CreateThought(ctx context.Context, thought thoughts.Thought) (thoughts.Thought, error) {
	const query = `
insert into thoughts (id, user_id, content, exposure_scope, user_tags, metadata, ingest_status, ingest_error, created_at, updated_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
returning id, user_id, content, exposure_scope, user_tags, coalesce(metadata, '{}'::jsonb), ingest_status, coalesce(embedding_model, ''), coalesce(ingest_error, ''), created_at, updated_at
`
	metadataJSON, err := marshalMetadata(thought.Metadata)
	if err != nil {
		return thoughts.Thought{}, err
	}
	return scanThought(s.db.QueryRow(ctx, query,
		thought.ID,
		thought.UserID,
		thought.Content,
		string(thought.ExposureScope),
		thought.UserTags,
		metadataJSON,
		string(thought.IngestStatus),
		thought.IngestError,
		thought.CreatedAt,
		thought.UpdatedAt,
	))
}

func (s *ThoughtStore) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	const query = `
select id, user_id, content, exposure_scope, user_tags, coalesce(metadata, '{}'::jsonb), ingest_status, coalesce(embedding_model, ''), coalesce(ingest_error, ''), created_at, updated_at
from thoughts
where user_id = $1 and id = $2
`
	thought, err := scanThought(s.db.QueryRow(ctx, query, userID, thoughtID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return thoughts.Thought{}, thoughts.ErrThoughtNotFound
		}
		return thoughts.Thought{}, err
	}
	return thought, nil
}

func (s *ThoughtStore) UpdateThought(ctx context.Context, params thoughts.UpdateThoughtParams) (thoughts.Thought, error) {
	const query = `
update thoughts
set content = $3, exposure_scope = $4, user_tags = $5, ingest_status = $6, ingest_error = $7, updated_at = $8
where user_id = $1 and id = $2
returning id, user_id, content, exposure_scope, user_tags, coalesce(metadata, '{}'::jsonb), ingest_status, coalesce(embedding_model, ''), coalesce(ingest_error, ''), created_at, updated_at
`
	thought, err := scanThought(s.db.QueryRow(ctx, query,
		params.UserID,
		params.ThoughtID,
		params.Content,
		string(params.ExposureScope),
		params.UserTags,
		string(params.IngestStatus),
		params.IngestError,
		params.UpdatedAt,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return thoughts.Thought{}, thoughts.ErrThoughtNotFound
		}
		return thoughts.Thought{}, err
	}
	return thought, nil
}

func (s *ThoughtStore) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	const query = `delete from thoughts where user_id = $1 and id = $2`
	tag, err := s.db.Exec(ctx, query, userID, thoughtID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return thoughts.ErrThoughtNotFound
	}
	return nil
}

func (s *ThoughtStore) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	const query = `
select id, user_id, content, exposure_scope, user_tags, coalesce(metadata, '{}'::jsonb), ingest_status, coalesce(embedding_model, ''), coalesce(ingest_error, ''), created_at, updated_at
from thoughts
where user_id = $1
order by updated_at desc, id desc
limit $2 offset $3
`
	return s.queryThoughts(ctx, query, params.UserID, normalizePageSize(params.PageSize), offset(params.Page, params.PageSize))
}

func (s *ThoughtStore) SearchKeyword(ctx context.Context, params thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	const query = `
select id, user_id, content, exposure_scope, user_tags, coalesce(metadata, '{}'::jsonb), ingest_status, coalesce(embedding_model, ''), coalesce(ingest_error, ''), created_at, updated_at
from thoughts
where user_id = $1 and content ilike '%' || $2 || '%'
order by updated_at desc, id desc
limit $3 offset $4
`
	return s.queryThoughts(ctx, query, params.UserID, params.Query, normalizePageSize(params.PageSize), offset(params.Page, params.PageSize))
}

func (s *ThoughtStore) SearchSemantic(ctx context.Context, params thoughts.SearchSemanticParams) ([]thoughts.Thought, error) {
	return s.SearchKeyword(ctx, thoughts.SearchKeywordParams{UserID: params.UserID, Query: params.Query, Page: params.Page, PageSize: params.PageSize})
}

func (s *ThoughtStore) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	const query = `
update thoughts
set ingest_status = $3, ingest_error = '', updated_at = now()
where user_id = $1 and id = $2
returning id, user_id, content, exposure_scope, user_tags, coalesce(metadata, '{}'::jsonb), ingest_status, coalesce(embedding_model, ''), coalesce(ingest_error, ''), created_at, updated_at
`
	thought, err := scanThought(s.db.QueryRow(ctx, query, userID, thoughtID, string(thoughts.IngestStatusPending)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return thoughts.Thought{}, thoughts.ErrThoughtNotFound
		}
		return thoughts.Thought{}, err
	}
	return thought, nil
}

func (s *ThoughtStore) ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error) {
	const query = `
select id, user_id, content, exposure_scope, user_tags, coalesce(metadata, '{}'::jsonb), ingest_status, coalesce(embedding_model, ''), coalesce(ingest_error, ''), created_at, updated_at
from thoughts
where ingest_status = $1
order by updated_at desc, id desc
limit $2
`
	return s.queryThoughts(ctx, query, string(thoughts.IngestStatusPending), limit)
}

func (s *ThoughtStore) MarkReady(ctx context.Context, params thoughts.MarkReadyParams) error {
	const query = `
update thoughts
set ingest_status = $2, ingest_error = '', metadata = $3, embedding = $4::vector, embedding_model = $5, updated_at = $6
where id = $1
`
	metadataJSON, err := marshalMetadata(params.Metadata)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, query, params.ThoughtID, string(thoughts.IngestStatusReady), metadataJSON, formatVector(params.Embedding), params.EmbeddingModel, params.ProcessedAt)
	return err
}

func (s *ThoughtStore) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	const query = `update thoughts set ingest_status = $2, ingest_error = $3, updated_at = now() where id = $1`
	_, err := s.db.Exec(ctx, query, id, string(thoughts.IngestStatusFailed), reason)
	return err
}

func (s *ThoughtStore) queryThoughts(ctx context.Context, query string, args ...any) ([]thoughts.Thought, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]thoughts.Thought, 0)
	for rows.Next() {
		thought, err := scanThought(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, thought)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func scanThought(row interface{ Scan(dest ...any) error }) (thoughts.Thought, error) {
	var thought thoughts.Thought
	var exposureScope string
	var metadataJSON []byte
	var ingestStatus string
	if err := row.Scan(
		&thought.ID,
		&thought.UserID,
		&thought.Content,
		&exposureScope,
		&thought.UserTags,
		&metadataJSON,
		&ingestStatus,
		&thought.EmbeddingModel,
		&thought.IngestError,
		&thought.CreatedAt,
		&thought.UpdatedAt,
	); err != nil {
		return thoughts.Thought{}, err
	}
	thought.ExposureScope = thoughts.ExposureScope(exposureScope)
	thought.IngestStatus = thoughts.IngestStatus(ingestStatus)
	thought.Metadata = decodeMetadata(metadataJSON)
	return thought, nil
}

func normalizePageSize(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	return pageSize
}

func offset(page, pageSize int) int {
	if page <= 1 {
		return 0
	}
	return (page - 1) * normalizePageSize(pageSize)
}

func marshalMetadata(value metadata.Metadata) ([]byte, error) {
	raw := map[string]any{
		"summary":  value.Summary,
		"topics":   value.Topics,
		"entities": value.Entities,
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return payload, nil
}

func decodeMetadata(payload []byte) metadata.Metadata {
	if len(payload) == 0 {
		return metadata.Normalize(nil)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return metadata.Normalize(nil)
	}
	return metadata.Normalize(raw)
}

func formatVector(values []float32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'f', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
