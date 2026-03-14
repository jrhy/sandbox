package query

import (
	"context"

	"github.com/google/uuid"
	"openbrainfun/internal/thoughts"
)

type ViewerType string

const (
	ViewerLocalUI   ViewerType = "local_ui"
	ViewerRemoteMCP ViewerType = "remote_mcp"
)

type Repository interface {
	GetThought(ctx context.Context, id uuid.UUID) (thoughts.Thought, error)
	ListRecent(ctx context.Context, params thoughts.ListRecentParams) ([]thoughts.Thought, error)
	SearchKeyword(ctx context.Context, params thoughts.SearchKeywordParams) ([]thoughts.Thought, error)
	SearchSemantic(ctx context.Context, params thoughts.SearchSemanticParams) ([]thoughts.Thought, error)
}

type Embedder interface {
	Embed(ctx context.Context, input []string) ([][]float32, error)
}

type Service struct {
	repo     Repository
	embedder Embedder
}

func NewService(repo Repository, embedder Embedder) *Service {
	return &Service{repo: repo, embedder: embedder}
}

func (s *Service) ListRecent(ctx context.Context, limit int, viewer ViewerType) ([]thoughts.Thought, error) {
	rows, err := s.repo.ListRecent(ctx, thoughts.ListRecentParams{Limit: limit})
	if err != nil {
		return nil, err
	}
	return filter(rows, viewer), nil
}

func (s *Service) SearchKeyword(ctx context.Context, q string, limit int, viewer ViewerType) ([]thoughts.Thought, error) {
	rows, err := s.repo.SearchKeyword(ctx, thoughts.SearchKeywordParams{Query: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	return filter(rows, viewer), nil
}

func (s *Service) SearchSemantic(ctx context.Context, q string, limit int, viewer ViewerType) ([]thoughts.Thought, error) {
	vecs, err := s.embedder.Embed(ctx, []string{q})
	if err != nil || len(vecs) == 0 {
		return nil, err
	}
	rows, err := s.repo.SearchSemantic(ctx, thoughts.SearchSemanticParams{Embedding: vecs[0], Limit: limit})
	if err != nil {
		return nil, err
	}
	return filter(rows, viewer), nil
}

func (s *Service) GetThought(ctx context.Context, id uuid.UUID, viewer ViewerType) (thoughts.Thought, bool, error) {
	t, err := s.repo.GetThought(ctx, id)
	if err != nil {
		return thoughts.Thought{}, false, err
	}
	if viewer == ViewerRemoteMCP && t.ExposureScope != thoughts.ExposureRemoteOK {
		return thoughts.Thought{}, false, nil
	}
	return t, true, nil
}

func filter(in []thoughts.Thought, viewer ViewerType) []thoughts.Thought {
	if viewer != ViewerRemoteMCP {
		return in
	}
	out := make([]thoughts.Thought, 0, len(in))
	for _, t := range in {
		if t.ExposureScope == thoughts.ExposureRemoteOK {
			out = append(out, t)
		}
	}
	return out
}
