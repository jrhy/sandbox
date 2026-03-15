package thoughts

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
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
	if current.Content != input.Content || current.ExposureScope != input.ExposureScope || !sameTags(current.UserTags, input.UserTags) {
		status = IngestStatusPending
		ingestError = ""
	}
	return s.repo.UpdateThought(ctx, UpdateThoughtParams{
		ThoughtID:     input.ThoughtID,
		UserID:        input.UserID,
		Content:       input.Content,
		ExposureScope: input.ExposureScope,
		UserTags:      input.UserTags,
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
	if params.SearchMode == SearchModeSemantic && params.Q != "" {
		return s.repo.SearchSemantic(ctx, SearchSemanticParams{UserID: params.UserID, Query: params.Q, Page: params.Page, PageSize: params.PageSize})
	}
	if params.Q != "" {
		return s.repo.SearchKeyword(ctx, SearchKeywordParams{UserID: params.UserID, Query: params.Q, Exposure: params.Exposure, IngestStatus: params.IngestStatus, Tag: params.Tag, Page: params.Page, PageSize: params.PageSize})
	}
	return s.repo.ListThoughts(ctx, params)
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
