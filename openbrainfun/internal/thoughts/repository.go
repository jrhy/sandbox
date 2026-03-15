package thoughts

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
)

var ErrThoughtNotFound = errors.New("thought not found")

type SearchMode string

const (
	SearchModeKeyword  SearchMode = "keyword"
	SearchModeSemantic SearchMode = "semantic"
)

type CreateThoughtInput struct {
	UserID        uuid.UUID
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
}

type ScoredThought struct {
	Thought    Thought
	Similarity float64
}

type UpdateThoughtInput struct {
	ThoughtID     uuid.UUID
	UserID        uuid.UUID
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
	ResetIngest   bool
	CurrentStatus IngestStatus
	CurrentError  string
}

type UpdateThoughtParams struct {
	ThoughtID     uuid.UUID
	UserID        uuid.UUID
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
	IngestStatus  IngestStatus
	IngestError   string
	UpdatedAt     time.Time
}

type ListThoughtsParams struct {
	UserID       uuid.UUID
	Q            string
	SearchMode   SearchMode
	Exposure     string
	IngestStatus string
	Tag          string
	Page         int
	PageSize     int
}

type SearchKeywordParams struct {
	UserID       uuid.UUID
	Query        string
	Exposure     string
	IngestStatus string
	Tag          string
	Page         int
	PageSize     int
}

type SearchSemanticParams struct {
	UserID         uuid.UUID
	QueryEmbedding []float32
	Threshold      float64
	Exposure       string
	Tag            string
	Page           int
	PageSize       int
}

type SearchThoughtsInput struct {
	UserID       uuid.UUID
	Query        string
	SearchMode   SearchMode
	Threshold    float64
	Exposure     string
	IngestStatus string
	Tag          string
	Page         int
	PageSize     int
}

type RelatedThoughtsInput struct {
	UserID    uuid.UUID
	ThoughtID uuid.UUID
	Exposure  string
	Limit     int
}

type RelatedThoughtsParams struct {
	UserID    uuid.UUID
	ThoughtID uuid.UUID
	Exposure  string
	Limit     int
}

type MarkReadyParams struct {
	ThoughtID      uuid.UUID
	Embedding      []float32
	EmbeddingModel string
	Metadata       metadata.Metadata
	ProcessedAt    time.Time
}

type Repository interface {
	CreateThought(ctx context.Context, thought Thought) (Thought, error)
	GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error)
	UpdateThought(ctx context.Context, params UpdateThoughtParams) (Thought, error)
	DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error
	ListThoughts(ctx context.Context, params ListThoughtsParams) ([]Thought, error)
	SearchKeyword(ctx context.Context, params SearchKeywordParams) ([]Thought, error)
	SearchSemantic(ctx context.Context, params SearchSemanticParams) ([]ScoredThought, error)
	RelatedThoughts(ctx context.Context, params RelatedThoughtsParams) ([]ScoredThought, error)
	RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error)
	ClaimPending(ctx context.Context, limit int) ([]Thought, error)
	MarkReady(ctx context.Context, params MarkReadyParams) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
}
