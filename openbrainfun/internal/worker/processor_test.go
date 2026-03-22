package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestProcessorMarksThoughtReadyAfterEmbeddingAndMetadataExtraction(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: uuid.New(), UserID: uuid.New(), Content: "remember mcp auth", EmbeddingStatus: thoughts.IngestStatusPending, MetadataStatus: thoughts.IngestStatusPending, IngestStatus: thoughts.IngestStatusPending}}}
	processor := NewProcessor(
		repo,
		embed.NewFake(map[string][]float32{"remember mcp auth": {0.1, 0.2, 0.3}}),
		metadata.NewFake(map[string]metadata.Metadata{
			"remember mcp auth": {Summary: "remember mcp auth", Topics: []string{"mcp", "auth"}},
		}),
	)

	if err := processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repo.processedCalls) != 1 {
		t.Fatalf("processedCalls = %d, want 1", len(repo.processedCalls))
	}
	if len(repo.processedCalls[0].Metadata.Topics) == 0 {
		t.Fatalf("expected extracted metadata to be persisted")
	}
}

func TestProcessorMarksThoughtReadyWithDefaultMetadataWhenExtractionFails(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: uuid.New(), UserID: uuid.New(), Content: "remember mcp auth", EmbeddingStatus: thoughts.IngestStatusPending, MetadataStatus: thoughts.IngestStatusPending, IngestStatus: thoughts.IngestStatusPending}}}
	processor := NewProcessor(
		repo,
		embed.NewFake(map[string][]float32{"remember mcp auth": {0.1, 0.2, 0.3}}),
		errorExtractor{err: errors.New("ollama chat failed")},
	)

	if err := processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repo.processedCalls) != 1 {
		t.Fatalf("processedCalls = %d, want 1", len(repo.processedCalls))
	}
	if repo.processedCalls[0].Metadata.Summary != "No summary available." {
		t.Fatalf("Summary = %q, want default summary", repo.processedCalls[0].Metadata.Summary)
	}
	if repo.processedCalls[0].MetadataError == "" {
		t.Fatal("MetadataError = empty, want recorded extraction error")
	}
}

func TestProcessorMarksThoughtFailedWhenEmbeddingFails(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: uuid.New(), UserID: uuid.New(), Content: "remember mcp auth", EmbeddingStatus: thoughts.IngestStatusPending, MetadataStatus: thoughts.IngestStatusPending, IngestStatus: thoughts.IngestStatusPending}}}
	processor := NewProcessor(
		repo,
		errorEmbedder{err: errors.New("ollama embed failed")},
		metadata.NewFake(nil),
	)

	if err := processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repo.failedCalls) != 1 {
		t.Fatalf("failedCalls = %d, want 1", len(repo.failedCalls))
	}
	if repo.failedCalls[0].Reason == "" {
		t.Fatal("reason = empty, want embed error message")
	}
}

type fakeRepo struct {
	pending        []thoughts.Thought
	processedCalls []thoughts.MarkProcessedParams
	failedCalls    []thoughts.MarkEmbeddingFailedParams
}

func (f *fakeRepo) ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error) {
	result := append([]thoughts.Thought(nil), f.pending...)
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeRepo) MarkProcessed(ctx context.Context, params thoughts.MarkProcessedParams) error {
	f.processedCalls = append(f.processedCalls, params)
	return nil
}

func (f *fakeRepo) MarkEmbeddingFailed(ctx context.Context, params thoughts.MarkEmbeddingFailedParams) error {
	f.failedCalls = append(f.failedCalls, params)
	return nil
}

type errorExtractor struct {
	err error
}

func (e errorExtractor) Extract(ctx context.Context, content string) (metadata.Metadata, error) {
	return metadata.Metadata{}, e.err
}

func (e errorExtractor) Model() string { return "error" }

func (e errorExtractor) Fingerprint() string { return "error|metadata:v1" }

type errorEmbedder struct {
	err error
}

func (e errorEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	return nil, e.err
}

func (e errorEmbedder) Dimensions() int { return 3 }

func (e errorEmbedder) Model() string { return "error" }

func (e errorEmbedder) Fingerprint() string { return "error|embed:v1" }
