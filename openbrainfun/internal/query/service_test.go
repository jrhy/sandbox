package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"openbrainfun/internal/thoughts"
)

type fakeRepo struct{ semanticResults []thoughts.Thought }

func (f *fakeRepo) GetThought(context.Context, uuid.UUID) (thoughts.Thought, error) {
	return thoughts.Thought{}, nil
}
func (f *fakeRepo) ListRecent(context.Context, thoughts.ListRecentParams) ([]thoughts.Thought, error) {
	return nil, nil
}
func (f *fakeRepo) SearchKeyword(context.Context, thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	return nil, nil
}
func (f *fakeRepo) SearchSemantic(context.Context, thoughts.SearchSemanticParams) ([]thoughts.Thought, error) {
	return f.semanticResults, nil
}

type fakeEmbedder struct{ vector []float32 }

func (f fakeEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{f.vector}, nil
}

func mustUUID(t *testing.T) uuid.UUID { id, _ := uuid.NewV7(); return id }

func TestSearchSemanticFiltersLocalOnlyForRemoteViewer(t *testing.T) {
	repo := &fakeRepo{semanticResults: []thoughts.Thought{
		{ID: mustUUID(t), Content: "local secret", ExposureScope: thoughts.ExposureLocalOnly},
		{ID: mustUUID(t), Content: "remote-safe note", ExposureScope: thoughts.ExposureRemoteOK},
	}}
	service := NewService(repo, fakeEmbedder{vector: []float32{0.1, 0.2, 0.3}})

	got, err := service.SearchSemantic(context.Background(), "note", 10, ViewerRemoteMCP)
	if err != nil {
		t.Fatalf("SearchSemantic() error = %v", err)
	}
	if len(got) != 1 || got[0].ExposureScope != thoughts.ExposureRemoteOK {
		t.Fatalf("got = %#v, want only remote-safe thought", got)
	}
}
