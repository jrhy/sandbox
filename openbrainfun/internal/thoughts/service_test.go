package thoughts

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDeleteThoughtRejectsWrongOwner(t *testing.T) {
	repo := &fakeRepo{thought: Thought{ID: uuid.New(), UserID: uuid.New(), Content: "secret"}, getErr: ErrThoughtNotFound}
	svc := NewService(repo)

	err := svc.DeleteThought(context.Background(), uuid.New(), repo.thought.ID)
	if !errors.Is(err, ErrThoughtNotFound) {
		t.Fatalf("error = %v, want ErrThoughtNotFound", err)
	}
}

func TestCreateThoughtStoresPendingRecord(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	userID := uuid.New()

	got, err := svc.CreateThought(context.Background(), CreateThoughtInput{
		UserID:        userID,
		Content:       "remember the auth flow",
		ExposureScope: ExposureScopeLocalOnly,
		UserTags:      []string{"auth"},
	})
	if err != nil {
		t.Fatalf("CreateThought() error = %v", err)
	}
	if got.IngestStatus != IngestStatusPending {
		t.Fatalf("IngestStatus = %q, want %q", got.IngestStatus, IngestStatusPending)
	}
	if repo.created.UserID != userID {
		t.Fatalf("created thought user = %s, want %s", repo.created.UserID, userID)
	}
}

func TestRetryThoughtResetsFailedIngest(t *testing.T) {
	repo := &fakeRepo{thought: Thought{
		ID:           uuid.New(),
		UserID:       uuid.New(),
		Content:      "secret",
		IngestStatus: IngestStatusFailed,
		IngestError:  "ollama down",
	}}
	svc := NewService(repo)

	got, err := svc.RetryThought(context.Background(), repo.thought.UserID, repo.thought.ID)
	if err != nil {
		t.Fatalf("RetryThought() error = %v", err)
	}
	if got.IngestStatus != IngestStatusPending || got.IngestError != "" {
		t.Fatalf("got %+v, want pending with cleared error", got)
	}
}

type fakeRepo struct {
	thought   Thought
	created   Thought
	getErr    error
	deleteErr error
}

func (f *fakeRepo) CreateThought(ctx context.Context, thought Thought) (Thought, error) {
	f.created = thought
	if thought.ID == uuid.Nil {
		thought.ID = uuid.New()
	}
	f.thought = thought
	return thought, nil
}

func (f *fakeRepo) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error) {
	if f.getErr != nil {
		return Thought{}, f.getErr
	}
	if f.thought.ID != thoughtID || f.thought.UserID != userID {
		return Thought{}, ErrThoughtNotFound
	}
	return f.thought, nil
}

func (f *fakeRepo) UpdateThought(ctx context.Context, params UpdateThoughtParams) (Thought, error) {
	return f.thought, nil
}

func (f *fakeRepo) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	return f.deleteErr
}

func (f *fakeRepo) ListThoughts(ctx context.Context, params ListThoughtsParams) ([]Thought, error) {
	return nil, nil
}

func (f *fakeRepo) SearchKeyword(ctx context.Context, params SearchKeywordParams) ([]Thought, error) {
	return nil, nil
}

func (f *fakeRepo) SearchSemantic(ctx context.Context, params SearchSemanticParams) ([]Thought, error) {
	return nil, nil
}

func (f *fakeRepo) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error) {
	if f.thought.ID != thoughtID || f.thought.UserID != userID {
		return Thought{}, ErrThoughtNotFound
	}
	f.thought.IngestStatus = IngestStatusPending
	f.thought.IngestError = ""
	return f.thought, nil
}

func (f *fakeRepo) ClaimPending(ctx context.Context, limit int) ([]Thought, error)    { return nil, nil }
func (f *fakeRepo) MarkReady(ctx context.Context, params MarkReadyParams) error       { return nil }
func (f *fakeRepo) MarkFailed(ctx context.Context, id uuid.UUID, reason string) error { return nil }
