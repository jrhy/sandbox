package thoughts

import (
	"context"

	"github.com/google/uuid"
)

type CreateThoughtParams struct {
	Thought Thought
}

type ListRecentParams struct {
	Limit int
}

type SearchKeywordParams struct {
	Query string
	Limit int
}

type SearchSemanticParams struct {
	Embedding []float32
	Limit     int
}

type MarkReadyParams struct {
	ID        uuid.UUID
	Embedding []float32
}

type Repository interface {
	CreateThought(ctx context.Context, params CreateThoughtParams) (Thought, error)
	GetThought(ctx context.Context, id uuid.UUID) (Thought, error)
	ListRecent(ctx context.Context, params ListRecentParams) ([]Thought, error)
	SearchKeyword(ctx context.Context, params SearchKeywordParams) ([]Thought, error)
	SearchSemantic(ctx context.Context, params SearchSemanticParams) ([]Thought, error)
	ClaimPending(ctx context.Context, limit int) ([]Thought, error)
	MarkReady(ctx context.Context, params MarkReadyParams) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
}
