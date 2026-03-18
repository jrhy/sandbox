package thoughts

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
)

type Service struct {
	repo     Repository
	embedder embed.Embedder
	now      func() time.Time
}

var ErrSearchUnavailable = errors.New("semantic search unavailable")

func NewService(repo Repository, embedder embed.Embedder) *Service {
	return &Service{repo: repo, embedder: embedder, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) CreateThought(ctx context.Context, input CreateThoughtInput) (Thought, error) {
	thought, err := NewThought(NewThoughtParams{
		UserID:        input.UserID,
		Content:       input.Content,
		ExposureScope: input.ExposureScope,
		UserTags:      input.UserTags,
	})
	if err != nil {
		return Thought{}, err
	}
	return s.repo.CreateThought(ctx, thought)
}

func (s *Service) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error) {
	return s.repo.GetThought(ctx, userID, thoughtID)
}

func (s *Service) UpdateThought(ctx context.Context, input UpdateThoughtInput) (Thought, error) {
	current, err := s.repo.GetThought(ctx, input.UserID, input.ThoughtID)
	if err != nil {
		return Thought{}, err
	}
	status := current.IngestStatus
	ingestError := current.IngestError
	normalizedTags := normalizeTags(input.UserTags)
	if current.Content != input.Content || current.ExposureScope != input.ExposureScope || !sameTags(current.UserTags, normalizedTags) {
		status = IngestStatusPending
		ingestError = ""
	}
	return s.repo.UpdateThought(ctx, UpdateThoughtParams{
		ThoughtID:     input.ThoughtID,
		UserID:        input.UserID,
		Content:       input.Content,
		ExposureScope: input.ExposureScope,
		UserTags:      normalizedTags,
		IngestStatus:  status,
		IngestError:   ingestError,
		UpdatedAt:     s.now(),
	})
}

func (s *Service) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	if _, err := s.repo.GetThought(ctx, userID, thoughtID); err != nil {
		return err
	}
	return s.repo.DeleteThought(ctx, userID, thoughtID)
}

func (s *Service) ListThoughts(ctx context.Context, params ListThoughtsParams) ([]Thought, error) {
	if params.Q != "" {
		return s.repo.SearchKeyword(ctx, SearchKeywordParams{UserID: params.UserID, Query: params.Q, Exposure: params.Exposure, IngestStatus: params.IngestStatus, Tag: params.Tag, Page: params.Page, PageSize: params.PageSize})
	}
	return s.repo.ListThoughts(ctx, params)
}

func (s *Service) SearchThoughts(ctx context.Context, input SearchThoughtsInput) ([]ScoredThought, error) {
	if input.SearchMode != SearchModeSemantic || input.Query == "" {
		return nil, nil
	}
	if s.embedder == nil {
		return nil, ErrSearchUnavailable
	}
	vectors, err := s.embedder.Embed(ctx, []string{input.Query})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return []ScoredThought{}, nil
	}
	return s.repo.SearchSemantic(ctx, SearchSemanticParams{
		UserID:         input.UserID,
		QueryEmbedding: vectors[0],
		Threshold:      input.Threshold,
		Exposure:       input.Exposure,
		Tag:            input.Tag,
		Page:           input.Page,
		PageSize:       input.PageSize,
	})
}

func (s *Service) RelatedThoughts(ctx context.Context, input RelatedThoughtsInput) ([]ScoredThought, error) {
	anchor, err := s.repo.GetThought(ctx, input.UserID, input.ThoughtID)
	if err != nil {
		return nil, err
	}
	if anchor.IngestStatus != IngestStatusReady {
		return []ScoredThought{}, nil
	}
	return s.repo.RelatedThoughts(ctx, RelatedThoughtsParams{
		UserID:    input.UserID,
		ThoughtID: input.ThoughtID,
		Exposure:  input.Exposure,
		Limit:     input.Limit,
	})
}

func (s *Service) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error) {
	thought, err := s.repo.GetThought(ctx, userID, thoughtID)
	if err != nil {
		return Thought{}, err
	}
	if thought.IngestStatus != IngestStatusFailed {
		return thought, nil
	}
	return s.repo.RetryThought(ctx, userID, thoughtID)
}

func sameTags(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
