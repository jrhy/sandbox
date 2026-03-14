package worker

import (
	"context"

	"github.com/google/uuid"
	"openbrainfun/internal/thoughts"
)

type Embedder interface {
	Embed(ctx context.Context, input []string) ([][]float32, error)
}

type Repository interface {
	ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error)
	MarkReady(ctx context.Context, params thoughts.MarkReadyParams) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
}

type Processor struct {
	repo     Repository
	embedder Embedder
}

func NewProcessor(repo Repository, embedder Embedder) *Processor {
	return &Processor{repo: repo, embedder: embedder}
}

func (p *Processor) RunOnce(ctx context.Context) error {
	items, err := p.repo.ClaimPending(ctx, 50)
	if err != nil {
		return err
	}
	for _, item := range items {
		vecs, err := p.embedder.Embed(ctx, []string{item.Content})
		if err != nil || len(vecs) == 0 {
			reason := "embed failed"
			if err != nil {
				reason = err.Error()
			}
			_ = p.repo.MarkFailed(ctx, item.ID, reason)
			continue
		}
		if err := p.repo.MarkReady(ctx, thoughts.MarkReadyParams{ID: item.ID, Embedding: vecs[0]}); err != nil {
			_ = p.repo.MarkFailed(ctx, item.ID, err.Error())
		}
	}
	return nil
}
