package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestSearchThoughtsReturnsOnlyMappedUsersRemoteOKThoughts(t *testing.T) {
	userID := uuid.New()
	svc := New(fakeQueryService{searchResults: []thoughts.ScoredThought{
		{Thought: thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "remote visible", ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusReady}, Similarity: 0.88},
		{Thought: thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "local secret", ExposureScope: thoughts.ExposureScopeLocalOnly, IngestStatus: thoughts.IngestStatusReady}, Similarity: 0.87},
		{Thought: thoughts.Thought{ID: uuid.New(), UserID: uuid.New(), Content: "wrong user", ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusReady}, Similarity: 0.86},
	}})

	got, err := svc.SearchThoughts(context.Background(), auth.User{ID: userID}, SearchThoughtsInput{Query: "remote", Threshold: 0.5})
	if err != nil {
		t.Fatalf("SearchThoughts() error = %v", err)
	}
	if len(got.Thoughts) != 1 || got.Thoughts[0].Content != "remote visible" || got.Thoughts[0].Similarity == nil || *got.Thoughts[0].Similarity != 0.88 {
		t.Fatalf("got %+v, want one remote_ok result", got)
	}
}

func TestRelatedThoughtsReturnsOnlyMappedUsersRemoteOKThoughts(t *testing.T) {
	userID := uuid.New()
	anchorID := uuid.New()
	svc := New(fakeQueryService{
		thought: thoughts.Thought{ID: anchorID, UserID: userID, Content: "anchor", ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusReady},
		related: []thoughts.ScoredThought{
			{Thought: thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "remote visible", ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusReady}, Similarity: 0.91},
			{Thought: thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "local secret", ExposureScope: thoughts.ExposureScopeLocalOnly, IngestStatus: thoughts.IngestStatusReady}, Similarity: 0.9},
			{Thought: thoughts.Thought{ID: uuid.New(), UserID: uuid.New(), Content: "wrong user", ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusReady}, Similarity: 0.89},
		},
	})

	got, err := svc.RelatedThoughts(context.Background(), auth.User{ID: userID}, RelatedThoughtsInput{ID: anchorID.String(), Limit: 5})
	if err != nil {
		t.Fatalf("RelatedThoughts() error = %v", err)
	}
	if len(got.Thoughts) != 1 || got.Thoughts[0].Content != "remote visible" || got.Thoughts[0].Similarity == nil || *got.Thoughts[0].Similarity != 0.91 {
		t.Fatalf("got %+v, want one remote_ok related result", got)
	}
}

func TestGetThoughtRejectsLocalOnlyThought(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	svc := New(fakeQueryService{thought: thoughts.Thought{ID: thoughtID, UserID: userID, Content: "local secret", ExposureScope: thoughts.ExposureScopeLocalOnly}})

	_, err := svc.GetThought(context.Background(), auth.User{ID: userID}, GetThoughtInput{ID: thoughtID.String()})
	if err == nil {
		t.Fatal("error = nil, want local-only thought rejection")
	}
}

func TestStatsCountsOnlyRemoteOKThoughts(t *testing.T) {
	userID := uuid.New()
	svc := New(fakeQueryService{list: []thoughts.Thought{
		{ID: uuid.New(), UserID: userID, ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusReady},
		{ID: uuid.New(), UserID: userID, ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusFailed},
		{ID: uuid.New(), UserID: userID, ExposureScope: thoughts.ExposureScopeLocalOnly, IngestStatus: thoughts.IngestStatusReady},
	}})

	got, err := svc.Stats(context.Background(), auth.User{ID: userID})
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if got.Total != 2 || got.Ready != 1 || got.Failed != 1 {
		t.Fatalf("got %+v, want counts for remote_ok thoughts only", got)
	}
}

func TestMCPMiddlewareRejectsRevokedToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer revoked-token")
	rr := httptest.NewRecorder()

	NewAuthMiddleware(fakeAuthService{tokenErr: auth.ErrTokenRevoked})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

type fakeAuthService struct {
	user     auth.User
	tokenErr error
}

func (f fakeAuthService) RequireMCPTokenUser(ctx context.Context, token string) (auth.User, error) {
	if f.tokenErr != nil {
		return auth.User{}, f.tokenErr
	}
	if f.user.ID == uuid.Nil {
		f.user = auth.User{ID: uuid.New(), Username: "alice"}
	}
	return f.user, nil
}

type fakeQueryService struct {
	list          []thoughts.Thought
	searchResults []thoughts.ScoredThought
	related       []thoughts.ScoredThought
	thought       thoughts.Thought
	listParams    []thoughts.ListThoughtsParams
	searchInput   thoughts.SearchThoughtsInput
	relatedInput  thoughts.RelatedThoughtsInput
}

func (f fakeQueryService) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	return append([]thoughts.Thought(nil), f.list...), nil
}

func (f fakeQueryService) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	if f.thought.ID != thoughtID || f.thought.UserID != userID {
		return thoughts.Thought{}, thoughts.ErrThoughtNotFound
	}
	return f.thought, nil
}

func (f fakeQueryService) SearchThoughts(ctx context.Context, input thoughts.SearchThoughtsInput) ([]thoughts.ScoredThought, error) {
	return append([]thoughts.ScoredThought(nil), f.searchResults...), nil
}

func (f fakeQueryService) RelatedThoughts(ctx context.Context, input thoughts.RelatedThoughtsInput) ([]thoughts.ScoredThought, error) {
	return append([]thoughts.ScoredThought(nil), f.related...), nil
}

func (f fakeQueryService) UpdateThought(ctx context.Context, input thoughts.UpdateThoughtInput) (thoughts.Thought, error) {
	return thoughts.Thought{}, thoughts.ErrThoughtNotFound
}

func (f fakeQueryService) CreateThought(ctx context.Context, input thoughts.CreateThoughtInput) (thoughts.Thought, error) {
	return thoughts.Thought{}, nil
}

func (f fakeQueryService) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	return nil
}

func (f fakeQueryService) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	return thoughts.Thought{}, nil
}

func (f fakeQueryService) SearchKeyword(ctx context.Context, params thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	return nil, nil
}

func (f fakeQueryService) ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error) {
	return nil, nil
}

func (f fakeQueryService) MarkProcessed(ctx context.Context, params thoughts.MarkProcessedParams) error {
	return nil
}
func (f fakeQueryService) MarkEmbeddingFailed(ctx context.Context, params thoughts.MarkEmbeddingFailedParams) error {
	return nil
}
func (f fakeQueryService) ReconcileModels(ctx context.Context, params thoughts.ReconcileModelsParams) (thoughts.ReconcileModelsResult, error) {
	return thoughts.ReconcileModelsResult{}, nil
}

var _ = time.Second
