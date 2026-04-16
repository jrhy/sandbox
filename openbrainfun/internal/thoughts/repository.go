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

func ParseSearchMode(raw string) (SearchMode, bool) {
	switch mode := SearchMode(raw); mode {
	case "":
		return SearchModeSemantic, true
	case SearchModeKeyword, SearchModeSemantic:
		return mode, true
	default:
		return "", false
	}
}

func NormalizeSearchMode(mode SearchMode) SearchMode {
	if mode == "" {
		return SearchModeSemantic
	}
	return mode
}

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
	ThoughtID       uuid.UUID
	UserID          uuid.UUID
	Content         string
	ExposureScope   ExposureScope
	UserTags        []string
	EmbeddingStatus IngestStatus
	EmbeddingError  string
	MetadataStatus  IngestStatus
	MetadataError   string
	IngestStatus    IngestStatus
	IngestError     string
	UpdatedAt       time.Time
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

type MarkProcessedParams struct {
	ThoughtID            uuid.UUID
	UpdateEmbedding      bool
	Embedding            []float32
	EmbeddingModel       string
	EmbeddingFingerprint string
	EmbeddingStatus      IngestStatus
	EmbeddingError       string
	UpdateMetadata       bool
	Metadata             metadata.Metadata
	MetadataModel        string
	MetadataFingerprint  string
	MetadataStatus       IngestStatus
	MetadataError        string
	IngestStatus         IngestStatus
	IngestError          string
	ProcessedAt          time.Time
}

type MarkEmbeddingFailedParams struct {
	ThoughtID uuid.UUID
	Reason    string
	FailedAt  time.Time
}

type ReconcileModelsParams struct {
	EmbeddingModel       string
	EmbeddingFingerprint string
	MetadataModel        string
	MetadataFingerprint  string
	ReconciledAt         time.Time
}

type ReconcileModelsResult struct {
	EmbeddingMarked int64
	MetadataMarked  int64
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
	MarkProcessed(ctx context.Context, params MarkProcessedParams) error
	MarkEmbeddingFailed(ctx context.Context, params MarkEmbeddingFailedParams) error
	ReconcileModels(ctx context.Context, params ReconcileModelsParams) (ReconcileModelsResult, error)
}
