package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

const defaultClaimLimit = 20

type repository interface {
	ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error)
	MarkProcessed(ctx context.Context, params thoughts.MarkProcessedParams) error
	MarkEmbeddingFailed(ctx context.Context, params thoughts.MarkEmbeddingFailedParams) error
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
	processedAt := p.now()
	params := thoughts.MarkProcessedParams{
		ThoughtID:    thought.ID,
		IngestStatus: thought.IngestStatus,
		IngestError:  thought.IngestError,
	}

	if thought.EmbeddingStatus == thoughts.IngestStatusPending {
		vectors, err := p.embedder.Embed(ctx, []string{thought.Content})
		if err != nil {
			if markErr := p.repo.MarkEmbeddingFailed(ctx, thoughts.MarkEmbeddingFailedParams{ThoughtID: thought.ID, Reason: err.Error(), FailedAt: processedAt}); markErr != nil {
				return fmt.Errorf("mark thought %s embedding failed: %w", thought.ID, markErr)
			}
			return nil
		}
		if len(vectors) != 1 {
			if markErr := p.repo.MarkEmbeddingFailed(ctx, thoughts.MarkEmbeddingFailedParams{ThoughtID: thought.ID, Reason: "embedder returned unexpected vector count", FailedAt: processedAt}); markErr != nil {
				return fmt.Errorf("mark thought %s embedding failed: %w", thought.ID, markErr)
			}
			return nil
		}
		params.UpdateEmbedding = true
		params.Embedding = vectors[0]
		params.EmbeddingModel = p.embedder.Model()
		params.EmbeddingFingerprint = p.embedder.Fingerprint()
		params.EmbeddingStatus = thoughts.IngestStatusReady
		params.EmbeddingError = ""
	}

	if thought.MetadataStatus == thoughts.IngestStatusPending {
		extracted, err := p.extractor.Extract(ctx, thought.Content)
		metadataError := ""
		if err != nil {
			extracted = metadata.Normalize(nil)
			metadataError = err.Error()
		}
		params.UpdateMetadata = true
		params.Metadata = extracted
		params.MetadataModel = p.extractor.Model()
		params.MetadataFingerprint = p.extractor.Fingerprint()
		params.MetadataStatus = thoughts.IngestStatusReady
		params.MetadataError = metadataError
	}

	if !params.UpdateEmbedding && !params.UpdateMetadata {
		return nil
	}
	if !params.UpdateEmbedding {
		params.EmbeddingStatus = thought.EmbeddingStatus
		params.EmbeddingError = thought.EmbeddingError
	}
	if !params.UpdateMetadata {
		params.MetadataStatus = thought.MetadataStatus
		params.MetadataError = thought.MetadataError
	}
	params.IngestStatus = overallStatus(params.EmbeddingStatus, params.MetadataStatus)
	params.IngestError = overallError(params.EmbeddingStatus, params.EmbeddingError, params.MetadataError)
	params.ProcessedAt = processedAt

	if err := p.repo.MarkProcessed(ctx, params); err != nil {
		return fmt.Errorf("mark thought %s processed: %w", thought.ID, err)
	}
	return nil
}

func overallStatus(embeddingStatus, metadataStatus thoughts.IngestStatus) thoughts.IngestStatus {
	if embeddingStatus == thoughts.IngestStatusFailed {
		return thoughts.IngestStatusFailed
	}
	if embeddingStatus == thoughts.IngestStatusReady && metadataStatus == thoughts.IngestStatusReady {
		return thoughts.IngestStatusReady
	}
	return thoughts.IngestStatusPending
}

func overallError(embeddingStatus thoughts.IngestStatus, embeddingError, metadataError string) string {
	if embeddingStatus == thoughts.IngestStatusFailed {
		return embeddingError
	}
	return metadataError
}
