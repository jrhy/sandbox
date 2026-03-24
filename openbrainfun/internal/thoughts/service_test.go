package thoughts

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
)

func TestDeleteThoughtRejectsWrongOwner(t *testing.T) {
	repo := &fakeRepo{thought: Thought{ID: uuid.New(), UserID: uuid.New(), Content: "secret"}, getErr: ErrThoughtNotFound}
	svc := NewService(repo, embed.NewFake(nil))

	err := svc.DeleteThought(context.Background(), uuid.New(), repo.thought.ID)
	if !errors.Is(err, ErrThoughtNotFound) {
		t.Fatalf("error = %v, want ErrThoughtNotFound", err)
	}
}

func TestCreateThoughtStoresPendingRecord(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, embed.NewFake(nil))
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

func TestCreateThoughtNormalizesNilTags(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, embed.NewFake(nil))

	_, err := svc.CreateThought(context.Background(), CreateThoughtInput{
		UserID:        uuid.New(),
		Content:       "remember the auth flow",
		ExposureScope: ExposureScopeLocalOnly,
		UserTags:      nil,
	})
	if err != nil {
		t.Fatalf("CreateThought() error = %v", err)
	}
	if repo.created.UserTags == nil {
		t.Fatal("created tags = nil, want empty slice")
	}
	if len(repo.created.UserTags) != 0 {
		t.Fatalf("len(created tags) = %d, want 0", len(repo.created.UserTags))
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
	svc := NewService(repo, embed.NewFake(nil))

	got, err := svc.RetryThought(context.Background(), repo.thought.UserID, repo.thought.ID)
	if err != nil {
		t.Fatalf("RetryThought() error = %v", err)
	}
	if got.IngestStatus != IngestStatusPending || got.IngestError != "" {
		t.Fatalf("got %+v, want pending with cleared error", got)
	}
}

func TestSearchThoughtsEmbedsQueryAndReturnsScoredMatches(t *testing.T) {
	repo := &fakeRepo{
		semanticResults: []ScoredThought{{
			Thought:    Thought{ID: uuid.New(), UserID: uuid.New(), Content: "career change note"},
			Similarity: 0.88,
		}},
	}
	embedder := embed.NewFake(map[string][]float32{"career change": {0.1, 0.2, 0.3}})
	svc := NewService(repo, embedder)
	userID := uuid.New()

	got, err := svc.SearchThoughts(context.Background(), SearchThoughtsInput{
		UserID:     userID,
		Query:      "career change",
		SearchMode: SearchModeSemantic,
		Threshold:  0.5,
		Exposure:   string(ExposureScopeRemoteOK),
		Tag:        "career",
		Page:       2,
		PageSize:   5,
	})
	if err != nil {
		t.Fatalf("SearchThoughts() error = %v", err)
	}
	if len(got) != 1 || got[0].Similarity != 0.88 {
		t.Fatalf("got = %+v, want scored semantic results", got)
	}
	if repo.semanticParams.UserID != userID || repo.semanticParams.Threshold != 0.5 {
		t.Fatalf("semantic params = %+v", repo.semanticParams)
	}
	if repo.semanticParams.Exposure != string(ExposureScopeRemoteOK) || repo.semanticParams.Tag != "career" || repo.semanticParams.Page != 2 || repo.semanticParams.PageSize != 5 {
		t.Fatalf("semantic params = %+v", repo.semanticParams)
	}
	if len(repo.semanticParams.QueryEmbedding) != 3 || repo.semanticParams.QueryEmbedding[0] != 0.1 {
		t.Fatalf("QueryEmbedding = %#v, want embedded query vector", repo.semanticParams.QueryEmbedding)
	}
}

func TestRelatedThoughtsReturnsEmptyForNonReadyAnchor(t *testing.T) {
	repo := &fakeRepo{thought: Thought{
		ID:              uuid.New(),
		UserID:          uuid.New(),
		Content:         "pending thought",
		EmbeddingStatus: IngestStatusPending,
		IngestStatus:    IngestStatusPending,
	}}
	svc := NewService(repo, embed.NewFake(nil))

	got, err := svc.RelatedThoughts(context.Background(), RelatedThoughtsInput{
		UserID:    repo.thought.UserID,
		ThoughtID: repo.thought.ID,
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("RelatedThoughts() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want no related thoughts for non-ready anchor", got)
	}
	if repo.relatedCalled {
		t.Fatal("RelatedThoughts repository call happened for non-ready anchor")
	}
}

func TestRelatedThoughtsUsesReadyEmbeddingStatus(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	repo := &fakeRepo{
		thought: Thought{
			ID:              thoughtID,
			UserID:          userID,
			Content:         "ready embedding",
			EmbeddingStatus: IngestStatusReady,
			IngestStatus:    IngestStatusPending,
		},
		relatedResults: []ScoredThought{{
			Thought:    Thought{ID: uuid.New(), UserID: userID, Content: "similar"},
			Similarity: 0.91,
		}},
	}
	svc := NewService(repo, embed.NewFake(nil))

	got, err := svc.RelatedThoughts(context.Background(), RelatedThoughtsInput{
		UserID:    userID,
		ThoughtID: thoughtID,
		Exposure:  string(ExposureScopeLocalOnly),
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("RelatedThoughts() error = %v", err)
	}
	if !repo.relatedCalled {
		t.Fatal("RelatedThoughts repository call did not happen for ready embedding")
	}
	if len(got) != 1 || got[0].Similarity != 0.91 {
		t.Fatalf("got = %+v, want related results", got)
	}
	if repo.relatedParams.UserID != userID || repo.relatedParams.ThoughtID != thoughtID {
		t.Fatalf("related params = %+v", repo.relatedParams)
	}
}

func TestUpdateThoughtNormalizesNilTags(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	repo := &fakeRepo{thought: Thought{
		ID:              thoughtID,
		UserID:          userID,
		Content:         "secret",
		ExposureScope:   ExposureScopeLocalOnly,
		UserTags:        []string{"auth"},
		EmbeddingStatus: IngestStatusReady,
		EmbeddingError:  "embed failed before",
		MetadataStatus:  IngestStatusReady,
		MetadataError:   "metadata failed before",
		IngestStatus:    IngestStatusReady,
		IngestError:     "ingest failed before",
	}}
	svc := NewService(repo, embed.NewFake(nil))

	_, err := svc.UpdateThought(context.Background(), UpdateThoughtInput{
		ThoughtID:     thoughtID,
		UserID:        userID,
		Content:       "secret",
		ExposureScope: ExposureScopeLocalOnly,
		UserTags:      nil,
	})
	if err != nil {
		t.Fatalf("UpdateThought() error = %v", err)
	}
	if repo.updated.UserTags == nil {
		t.Fatal("updated tags = nil, want empty slice")
	}
	if len(repo.updated.UserTags) != 0 {
		t.Fatalf("len(updated tags) = %d, want 0", len(repo.updated.UserTags))
	}
	if repo.updated.EmbeddingStatus != IngestStatusPending {
		t.Fatalf("EmbeddingStatus = %q, want pending", repo.updated.EmbeddingStatus)
	}
	if repo.updated.EmbeddingError != "" {
		t.Fatalf("EmbeddingError = %q, want empty", repo.updated.EmbeddingError)
	}
	if repo.updated.MetadataStatus != IngestStatusPending {
		t.Fatalf("MetadataStatus = %q, want pending", repo.updated.MetadataStatus)
	}
	if repo.updated.MetadataError != "" {
		t.Fatalf("MetadataError = %q, want empty", repo.updated.MetadataError)
	}
	if repo.updated.IngestStatus != IngestStatusPending {
		t.Fatalf("IngestStatus = %q, want pending", repo.updated.IngestStatus)
	}
	if repo.updated.IngestError != "" {
		t.Fatalf("IngestError = %q, want empty", repo.updated.IngestError)
	}
}

type fakeRepo struct {
	thought         Thought
	created         Thought
	updated         UpdateThoughtParams
	getErr          error
	deleteErr       error
	semanticResults []ScoredThought
	semanticParams  SearchSemanticParams
	relatedResults  []ScoredThought
	relatedParams   RelatedThoughtsParams
	relatedCalled   bool
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
	f.updated = params
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

func (f *fakeRepo) SearchSemantic(ctx context.Context, params SearchSemanticParams) ([]ScoredThought, error) {
	f.semanticParams = params
	return append([]ScoredThought(nil), f.semanticResults...), nil
}

func (f *fakeRepo) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error) {
	if f.thought.ID != thoughtID || f.thought.UserID != userID {
		return Thought{}, ErrThoughtNotFound
	}
	f.thought.IngestStatus = IngestStatusPending
	f.thought.IngestError = ""
	return f.thought, nil
}

func (f *fakeRepo) RelatedThoughts(ctx context.Context, params RelatedThoughtsParams) ([]ScoredThought, error) {
	f.relatedCalled = true
	f.relatedParams = params
	return append([]ScoredThought(nil), f.relatedResults...), nil
}

func (f *fakeRepo) ClaimPending(ctx context.Context, limit int) ([]Thought, error)      { return nil, nil }
func (f *fakeRepo) MarkProcessed(ctx context.Context, params MarkProcessedParams) error { return nil }
func (f *fakeRepo) MarkEmbeddingFailed(ctx context.Context, params MarkEmbeddingFailedParams) error {
	return nil
}
func (f *fakeRepo) ReconcileModels(ctx context.Context, params ReconcileModelsParams) (ReconcileModelsResult, error) {
	return ReconcileModelsResult{}, nil
}
