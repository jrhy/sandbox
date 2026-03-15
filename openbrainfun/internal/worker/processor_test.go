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
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: uuid.New(), UserID: uuid.New(), Content: "remember mcp auth", IngestStatus: thoughts.IngestStatusPending}}}
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
	if len(repo.readyCalls) != 1 {
		t.Fatalf("readyCalls = %d, want 1", len(repo.readyCalls))
	}
	if len(repo.readyCalls[0].Metadata.Topics) == 0 {
		t.Fatalf("expected extracted metadata to be persisted")
	}
}

func TestProcessorMarksThoughtReadyWithDefaultMetadataWhenExtractionFails(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: uuid.New(), UserID: uuid.New(), Content: "remember mcp auth", IngestStatus: thoughts.IngestStatusPending}}}
	processor := NewProcessor(
		repo,
		embed.NewFake(map[string][]float32{"remember mcp auth": {0.1, 0.2, 0.3}}),
		errorExtractor{err: errors.New("ollama chat failed")},
	)

	if err := processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repo.readyCalls) != 1 {
		t.Fatalf("readyCalls = %d, want 1", len(repo.readyCalls))
	}
	if repo.readyCalls[0].Metadata.Summary != "No summary available." {
		t.Fatalf("Summary = %q, want default summary", repo.readyCalls[0].Metadata.Summary)
	}
}

func TestProcessorMarksThoughtFailedWhenEmbeddingFails(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: uuid.New(), UserID: uuid.New(), Content: "remember mcp auth", IngestStatus: thoughts.IngestStatusPending}}}
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
	if repo.failedCalls[0].reason == "" {
		t.Fatal("reason = empty, want embed error message")
	}
}

type fakeRepo struct {
	pending     []thoughts.Thought
	readyCalls  []thoughts.MarkReadyParams
	failedCalls []failedCall
}

type failedCall struct {
	id     uuid.UUID
	reason string
}

func (f *fakeRepo) ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error) {
	result := append([]thoughts.Thought(nil), f.pending...)
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (f *fakeRepo) MarkReady(ctx context.Context, params thoughts.MarkReadyParams) error {
	f.readyCalls = append(f.readyCalls, params)
	return nil
}

func (f *fakeRepo) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error {
	f.failedCalls = append(f.failedCalls, failedCall{id: id, reason: reason})
	return nil
}

type errorExtractor struct {
	err error
}

func (e errorExtractor) Extract(ctx context.Context, content string) (metadata.Metadata, error) {
	return metadata.Metadata{}, e.err
}

type errorEmbedder struct {
	err error
}

func (e errorEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	return nil, e.err
}

func (e errorEmbedder) Dimensions() int { return 3 }

func (e errorEmbedder) Model() string { return "error" }
