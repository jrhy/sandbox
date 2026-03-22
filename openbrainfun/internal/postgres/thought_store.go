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

func thoughtColumns(alias string) string {
	prefix := alias
	if prefix != "" {
		prefix += "."
	}
	return strings.Join([]string{
		prefix + "id",
		prefix + "user_id",
		prefix + "content",
		prefix + "exposure_scope",
		prefix + "user_tags",
		"coalesce(" + prefix + "metadata, '{}'::jsonb)",
		"coalesce(" + prefix + "embedding_model, '')",
		"coalesce(" + prefix + "embedding_fingerprint, '')",
		prefix + "embedding_status",
		"coalesce(" + prefix + "embedding_error, '')",
		"coalesce(" + prefix + "metadata_model, '')",
		"coalesce(" + prefix + "metadata_fingerprint, '')",
		prefix + "metadata_status",
		"coalesce(" + prefix + "metadata_error, '')",
		prefix + "ingest_status",
		"coalesce(" + prefix + "ingest_error, '')",
		prefix + "created_at",
		prefix + "updated_at",
	}, ",\n")
}

func (s *ThoughtStore) CreateThought(ctx context.Context, thought thoughts.Thought) (thoughts.Thought, error) {
	query := `
insert into thoughts (
	id, user_id, content, exposure_scope, user_tags, metadata,
	embedding_model, embedding_fingerprint, embedding_status, embedding_error,
	metadata_model, metadata_fingerprint, metadata_status, metadata_error,
	ingest_status, ingest_error, created_at, updated_at)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
returning ` + thoughtColumns("")
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
		thought.EmbeddingModel,
		thought.EmbeddingFingerprint,
		string(thought.EmbeddingStatus),
		thought.EmbeddingError,
		thought.MetadataModel,
		thought.MetadataFingerprint,
		string(thought.MetadataStatus),
		thought.MetadataError,
		string(thought.IngestStatus),
		thought.IngestError,
		thought.CreatedAt,
		thought.UpdatedAt,
	))
}

func (s *ThoughtStore) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	query := `select ` + thoughtColumns("") + ` from thoughts where user_id = $1 and id = $2`
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
	query := `
update thoughts
set content = $3,
    exposure_scope = $4,
    user_tags = $5,
    embedding_status = $6,
    embedding_error = $7,
    metadata_status = $8,
    metadata_error = $9,
    ingest_status = $10,
    ingest_error = $11,
    updated_at = $12
where user_id = $1 and id = $2
returning ` + thoughtColumns("")
	thought, err := scanThought(s.db.QueryRow(ctx, query,
		params.UserID,
		params.ThoughtID,
		params.Content,
		string(params.ExposureScope),
		params.UserTags,
		string(params.EmbeddingStatus),
		params.EmbeddingError,
		string(params.MetadataStatus),
		params.MetadataError,
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
	query := `select ` + thoughtColumns("") + ` from thoughts where user_id = $1 order by updated_at desc, id desc limit $2 offset $3`
	return s.queryThoughts(ctx, query, params.UserID, normalizePageSize(params.PageSize), offset(params.Page, params.PageSize))
}

func (s *ThoughtStore) SearchKeyword(ctx context.Context, params thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	query := `select ` + thoughtColumns("") + ` from thoughts where user_id = $1 and content ilike '%' || $2 || '%' order by updated_at desc, id desc limit $3 offset $4`
	return s.queryThoughts(ctx, query, params.UserID, params.Query, normalizePageSize(params.PageSize), offset(params.Page, params.PageSize))
}

func (s *ThoughtStore) SearchSemantic(ctx context.Context, params thoughts.SearchSemanticParams) ([]thoughts.ScoredThought, error) {
	query := `
select ` + thoughtColumns("") + `,
       1 - (embedding <=> $2::vector) as similarity
from thoughts
where user_id = $1
  and embedding_status = $3
  and embedding is not null
  and ($4 = '' or exposure_scope = $4)
  and ($5 = '' or $5 = any(user_tags))
  and ($6 <= 0 or 1 - (embedding <=> $2::vector) > $6)
order by embedding <=> $2::vector, id desc
limit $7 offset $8`
	return s.queryScoredThoughts(
		ctx,
		query,
		params.UserID,
		formatVector(params.QueryEmbedding),
		string(thoughts.IngestStatusReady),
		params.Exposure,
		params.Tag,
		params.Threshold,
		normalizePageSize(params.PageSize),
		offset(params.Page, params.PageSize),
	)
}

func (s *ThoughtStore) RelatedThoughts(ctx context.Context, params thoughts.RelatedThoughtsParams) ([]thoughts.ScoredThought, error) {
	query := `
with anchor as (
  select embedding
  from thoughts
  where user_id = $1 and id = $2 and embedding_status = $3 and embedding is not null
)
select ` + thoughtColumns("") + `,
       1 - (t.embedding <=> anchor.embedding) as similarity
from anchor
join thoughts t on t.user_id = $1
where t.id <> $2
  and t.embedding_status = $3
  and t.embedding is not null
  and ($4 = '' or t.exposure_scope = $4)
order by t.embedding <=> anchor.embedding, t.id desc
limit $5`
	return s.queryScoredThoughts(ctx, query, params.UserID, params.ThoughtID, string(thoughts.IngestStatusReady), params.Exposure, normalizePageSize(params.Limit))
}

func (s *ThoughtStore) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	query := `
update thoughts
set embedding_status = $3,
    embedding_error = '',
    metadata_status = $3,
    metadata_error = '',
    ingest_status = $3,
    ingest_error = '',
    updated_at = now()
where user_id = $1 and id = $2
returning ` + thoughtColumns("")
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
	query := `
select ` + thoughtColumns("") + `
from thoughts
where embedding_status = $1 or metadata_status = $1
order by updated_at desc, id desc
limit $2`
	return s.queryThoughts(ctx, query, string(thoughts.IngestStatusPending), limit)
}

func (s *ThoughtStore) MarkProcessed(ctx context.Context, params thoughts.MarkProcessedParams) error {
	setClauses := []string{"updated_at = $2", "ingest_status = $3", "ingest_error = $4"}
	args := []any{params.ThoughtID, params.ProcessedAt, string(params.IngestStatus), params.IngestError}
	index := 5
	if params.UpdateEmbedding {
		setClauses = append(setClauses,
			fmt.Sprintf("embedding = $%d::vector", index),
			fmt.Sprintf("embedding_model = $%d", index+1),
			fmt.Sprintf("embedding_fingerprint = $%d", index+2),
			fmt.Sprintf("embedding_status = $%d", index+3),
			fmt.Sprintf("embedding_error = $%d", index+4),
			fmt.Sprintf("embedding_updated_at = $%d", index+5),
		)
		args = append(args, formatVector(params.Embedding), params.EmbeddingModel, params.EmbeddingFingerprint, string(params.EmbeddingStatus), params.EmbeddingError, params.ProcessedAt)
		index += 6
	}
	if params.UpdateMetadata {
		metadataJSON, err := marshalMetadata(params.Metadata)
		if err != nil {
			return err
		}
		setClauses = append(setClauses,
			fmt.Sprintf("metadata = $%d", index),
			fmt.Sprintf("metadata_model = $%d", index+1),
			fmt.Sprintf("metadata_fingerprint = $%d", index+2),
			fmt.Sprintf("metadata_status = $%d", index+3),
			fmt.Sprintf("metadata_error = $%d", index+4),
			fmt.Sprintf("metadata_updated_at = $%d", index+5),
		)
		args = append(args, metadataJSON, params.MetadataModel, params.MetadataFingerprint, string(params.MetadataStatus), params.MetadataError, params.ProcessedAt)
	}
	query := `update thoughts set ` + strings.Join(setClauses, ", ") + ` where id = $1`
	_, err := s.db.Exec(ctx, query, args...)
	return err
}

func (s *ThoughtStore) MarkEmbeddingFailed(ctx context.Context, params thoughts.MarkEmbeddingFailedParams) error {
	const query = `
update thoughts
set embedding_status = $2,
    embedding_error = $3,
    ingest_status = $2,
    ingest_error = $3,
    updated_at = $4
where id = $1`
	_, err := s.db.Exec(ctx, query, params.ThoughtID, string(thoughts.IngestStatusFailed), params.Reason, params.FailedAt)
	return err
}

func (s *ThoughtStore) ReconcileModels(ctx context.Context, params thoughts.ReconcileModelsParams) (thoughts.ReconcileModelsResult, error) {
	var result thoughts.ReconcileModelsResult
	const embedQuery = `
update thoughts
set embedding_status = $3,
    embedding_error = '',
    ingest_status = $3,
    ingest_error = '',
    updated_at = $4
where coalesce(embedding_model, '') <> $1
   or coalesce(embedding_fingerprint, '') <> $2`
	embedTag, err := s.db.Exec(ctx, embedQuery, params.EmbeddingModel, params.EmbeddingFingerprint, string(thoughts.IngestStatusPending), params.ReconciledAt)
	if err != nil {
		return result, err
	}
	result.EmbeddingMarked = embedTag.RowsAffected()

	const metadataQuery = `
update thoughts
set metadata_status = $3,
    metadata_error = '',
    ingest_status = case when embedding_status = $4 then ingest_status else $3 end,
    updated_at = $5
where coalesce(metadata_model, '') <> $1
   or coalesce(metadata_fingerprint, '') <> $2`
	metadataTag, err := s.db.Exec(ctx, metadataQuery, params.MetadataModel, params.MetadataFingerprint, string(thoughts.IngestStatusPending), string(thoughts.IngestStatusFailed), params.ReconciledAt)
	if err != nil {
		return result, err
	}
	result.MetadataMarked = metadataTag.RowsAffected()
	return result, nil
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

func (s *ThoughtStore) queryScoredThoughts(ctx context.Context, query string, args ...any) ([]thoughts.ScoredThought, error) {
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]thoughts.ScoredThought, 0)
	for rows.Next() {
		item, err := scanScoredThought(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
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
	var embeddingStatus string
	var metadataStatus string
	var ingestStatus string
	if err := row.Scan(
		&thought.ID,
		&thought.UserID,
		&thought.Content,
		&exposureScope,
		&thought.UserTags,
		&metadataJSON,
		&thought.EmbeddingModel,
		&thought.EmbeddingFingerprint,
		&embeddingStatus,
		&thought.EmbeddingError,
		&thought.MetadataModel,
		&thought.MetadataFingerprint,
		&metadataStatus,
		&thought.MetadataError,
		&ingestStatus,
		&thought.IngestError,
		&thought.CreatedAt,
		&thought.UpdatedAt,
	); err != nil {
		return thoughts.Thought{}, err
	}
	thought.ExposureScope = thoughts.ExposureScope(exposureScope)
	thought.EmbeddingStatus = thoughts.IngestStatus(embeddingStatus)
	thought.MetadataStatus = thoughts.IngestStatus(metadataStatus)
	thought.IngestStatus = thoughts.IngestStatus(ingestStatus)
	thought.Metadata = decodeMetadata(metadataJSON)
	return thought, nil
}

func scanScoredThought(row interface{ Scan(dest ...any) error }) (thoughts.ScoredThought, error) {
	thought, similarity, err := scanThoughtWithSimilarity(row)
	if err != nil {
		return thoughts.ScoredThought{}, err
	}
	return thoughts.ScoredThought{Thought: thought, Similarity: similarity}, nil
}

func scanThoughtWithSimilarity(row interface{ Scan(dest ...any) error }) (thoughts.Thought, float64, error) {
	var thought thoughts.Thought
	var exposureScope string
	var metadataJSON []byte
	var embeddingStatus string
	var metadataStatus string
	var ingestStatus string
	var similarity float64
	if err := row.Scan(
		&thought.ID,
		&thought.UserID,
		&thought.Content,
		&exposureScope,
		&thought.UserTags,
		&metadataJSON,
		&thought.EmbeddingModel,
		&thought.EmbeddingFingerprint,
		&embeddingStatus,
		&thought.EmbeddingError,
		&thought.MetadataModel,
		&thought.MetadataFingerprint,
		&metadataStatus,
		&thought.MetadataError,
		&ingestStatus,
		&thought.IngestError,
		&thought.CreatedAt,
		&thought.UpdatedAt,
		&similarity,
	); err != nil {
		return thoughts.Thought{}, 0, err
	}
	thought.ExposureScope = thoughts.ExposureScope(exposureScope)
	thought.EmbeddingStatus = thoughts.IngestStatus(embeddingStatus)
	thought.MetadataStatus = thoughts.IngestStatus(metadataStatus)
	thought.IngestStatus = thoughts.IngestStatus(ingestStatus)
	thought.Metadata = decodeMetadata(metadataJSON)
	return thought, similarity, nil
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
