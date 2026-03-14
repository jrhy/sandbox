package thoughts

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type fakeRepo struct{ created []Thought }

func (f *fakeRepo) CreateThought(_ context.Context, params CreateThoughtParams) (Thought, error) {
	f.created = append(f.created, params.Thought)
	return params.Thought, nil
}
func (f *fakeRepo) GetThought(context.Context, uuid.UUID) (Thought, error)          { return Thought{}, nil }
func (f *fakeRepo) ListRecent(context.Context, ListRecentParams) ([]Thought, error) { return nil, nil }
func (f *fakeRepo) SearchKeyword(context.Context, SearchKeywordParams) ([]Thought, error) {
	return nil, nil
}
func (f *fakeRepo) SearchSemantic(context.Context, SearchSemanticParams) ([]Thought, error) {
	return nil, nil
}
func (f *fakeRepo) ClaimPending(context.Context, int) ([]Thought, error) { return nil, nil }
func (f *fakeRepo) MarkReady(context.Context, MarkReadyParams) error     { return nil }
func (f *fakeRepo) MarkFailed(context.Context, uuid.UUID, string) error  { return nil }

func TestCreateThoughtStoresPendingRecord(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	thought, err := svc.CreateThought(context.Background(), CreateThoughtInput{
		Content:       "Need to revisit the MCP auth flow",
		ExposureScope: ExposureRemoteOK,
		UserTags:      []string{"architecture"},
	})
	if err != nil {
		t.Fatalf("CreateThought() error = %v", err)
	}
	if thought.IngestStatus != IngestPending {
		t.Fatalf("IngestStatus = %q, want pending", thought.IngestStatus)
	}
}
