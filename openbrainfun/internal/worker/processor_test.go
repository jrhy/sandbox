package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"openbrainfun/internal/thoughts"
)

type fakeRepo struct {
	pending    []thoughts.Thought
	readyCalls []thoughts.MarkReadyParams
}

func (f *fakeRepo) ClaimPending(context.Context, int) ([]thoughts.Thought, error) {
	return f.pending, nil
}
func (f *fakeRepo) MarkReady(_ context.Context, p thoughts.MarkReadyParams) error {
	f.readyCalls = append(f.readyCalls, p)
	return nil
}
func (f *fakeRepo) MarkFailed(context.Context, uuid.UUID, string) error { return nil }

type fakeEmbedder struct{ vector []float32 }

func (f *fakeEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{f.vector}, nil
}

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestProcessorMarksThoughtReadyAfterEmbedding(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: mustUUID(t), Content: "remember to add retry visibility", IngestStatus: thoughts.IngestPending}}}
	embedder := &fakeEmbedder{vector: []float32{0.1, 0.2, 0.3}}
	processor := NewProcessor(repo, embedder)

	if err := processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repo.readyCalls) != 1 {
		t.Fatalf("readyCalls = %d, want 1", len(repo.readyCalls))
	}
}
