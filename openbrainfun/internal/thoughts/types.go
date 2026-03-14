package thoughts

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ExposureScope string

const (
	ExposureLocalOnly ExposureScope = "local_only"
	ExposureRemoteOK  ExposureScope = "remote_ok"
)

type IngestStatus string

const (
	IngestPending IngestStatus = "pending"
	IngestReady   IngestStatus = "ready"
	IngestFailed  IngestStatus = "failed"
)

type Thought struct {
	ID            uuid.UUID
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
	Source        string
	IngestStatus  IngestStatus
	FailureReason string
	Embedding     []float32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type NewThoughtParams struct {
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
	Source        string
}

func NewThought(params NewThoughtParams) (Thought, error) {
	content := strings.TrimSpace(params.Content)
	if content == "" {
		return Thought{}, errors.New("content is required")
	}
	scope := params.ExposureScope
	if scope == "" {
		scope = ExposureLocalOnly
	}
	if scope != ExposureLocalOnly && scope != ExposureRemoteOK {
		return Thought{}, errors.New("invalid exposure scope")
	}
	now := time.Now().UTC()
	return Thought{
		ID:            uuid.New(),
		Content:       content,
		ExposureScope: scope,
		UserTags:      append([]string(nil), params.UserTags...),
		Source:        params.Source,
		IngestStatus:  IngestPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}
