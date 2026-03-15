package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

const defaultClaimLimit = 20

type repository interface {
	ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error)
	MarkReady(ctx context.Context, params thoughts.MarkReadyParams) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
}

type Processor struct {
	repo       repository
	embedder   embed.Embedder
	extractor  metadata.Extractor
	claimLimit int
	now        func() time.Time
}

func NewProcessor(repo repository, embedder embed.Embedder, extractor metadata.Extractor) *Processor {
	return &Processor{
		repo:       repo,
		embedder:   embedder,
		extractor:  extractor,
		claimLimit: defaultClaimLimit,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (p *Processor) RunOnce(ctx context.Context) error {
	pending, err := p.repo.ClaimPending(ctx, p.claimLimit)
	if err != nil {
		return fmt.Errorf("claim pending thoughts: %w", err)
	}

	for _, thought := range pending {
		if err := p.processThought(ctx, thought); err != nil {
			return err
		}
	}
	return nil
}

func (p *Processor) processThought(ctx context.Context, thought thoughts.Thought) error {
	vectors, err := p.embedder.Embed(ctx, []string{thought.Content})
	if err != nil {
		if markErr := p.repo.MarkFailed(ctx, thought.ID, err.Error()); markErr != nil {
			return fmt.Errorf("mark thought %s failed: %w", thought.ID, markErr)
		}
		return nil
	}
	if len(vectors) != 1 {
		if markErr := p.repo.MarkFailed(ctx, thought.ID, "embedder returned unexpected vector count"); markErr != nil {
			return fmt.Errorf("mark thought %s failed: %w", thought.ID, markErr)
		}
		return nil
	}

	extracted, err := p.extractor.Extract(ctx, thought.Content)
	if err != nil {
		extracted = metadata.Normalize(nil)
	}

	if err := p.repo.MarkReady(ctx, thoughts.MarkReadyParams{
		ThoughtID:      thought.ID,
		Embedding:      vectors[0],
		EmbeddingModel: p.embedder.Model(),
		Metadata:       extracted,
		ProcessedAt:    p.now(),
	}); err != nil {
		return fmt.Errorf("mark thought %s ready: %w", thought.ID, err)
	}
	return nil
}
