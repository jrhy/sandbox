package thoughts

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
)

var (
	ErrBlankContent    = errors.New("blank content")
	ErrInvalidExposure = errors.New("invalid exposure scope")
)

type ExposureScope string

const (
	ExposureScopeLocalOnly ExposureScope = "local_only"
	ExposureScopeRemoteOK  ExposureScope = "remote_ok"
)

type IngestStatus string

const (
	IngestStatusPending IngestStatus = "pending"
	IngestStatusReady   IngestStatus = "ready"
	IngestStatusFailed  IngestStatus = "failed"
)

type Thought struct {
	ID                   uuid.UUID
	UserID               uuid.UUID
	Content              string
	ExposureScope        ExposureScope
	UserTags             []string
	Metadata             metadata.Metadata
	Embedding            []float32
	EmbeddingModel       string
	EmbeddingFingerprint string
	EmbeddingStatus      IngestStatus
	EmbeddingError       string
	MetadataModel        string
	MetadataFingerprint  string
	MetadataStatus       IngestStatus
	MetadataError        string
	IngestStatus         IngestStatus
	IngestError          string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type NewThoughtParams struct {
	UserID        uuid.UUID
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
}

func NewThought(params NewThoughtParams) (Thought, error) {
	trimmedContent := strings.TrimSpace(params.Content)
	if trimmedContent == "" {
		return Thought{}, ErrBlankContent
	}
	if !params.ExposureScope.Valid() {
		return Thought{}, ErrInvalidExposure
	}

	tags := normalizeTags(params.UserTags)
	now := time.Now().UTC()
	return Thought{
		ID:              uuid.New(),
		UserID:          params.UserID,
		Content:         trimmedContent,
		ExposureScope:   params.ExposureScope,
		UserTags:        tags,
		Metadata:        metadata.Normalize(nil),
		EmbeddingStatus: IngestStatusPending,
		MetadataStatus:  IngestStatusPending,
		IngestStatus:    IngestStatusPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{}
	}
	return append([]string{}, tags...)
}

func (scope ExposureScope) Valid() bool {
	switch scope {
	case ExposureScopeLocalOnly, ExposureScopeRemoteOK:
		return true
	default:
		return false
	}
}
