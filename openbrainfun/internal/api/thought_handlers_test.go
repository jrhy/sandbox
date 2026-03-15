package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestPostThoughtReturnsCreatedJSON(t *testing.T) {
	service := &fakeThoughtService{created: thoughts.Thought{ID: uuid.New(), Content: "Remember MCP auth", ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusPending}}
	handlers := NewThoughtHandlers(service)
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodPost, "/api/thoughts", bytes.NewBufferString(`{"content":"Remember MCP auth","exposure_scope":"remote_ok","user_tags":["mcp"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
	if service.createInput.UserID != user.ID {
		t.Fatalf("createInput.UserID = %s, want %s", service.createInput.UserID, user.ID)
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got["content"] != "Remember MCP auth" {
		t.Fatalf("content = %v, want created thought", got["content"])
	}
}

func TestGetThoughtReturnsJSON(t *testing.T) {
	thoughtID := uuid.New()
	handlers := NewThoughtHandlers(&fakeThoughtService{thought: thoughts.Thought{ID: thoughtID, Content: "remember pgx", Metadata: metadata.Metadata{Summary: "remember pgx"}}})
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodGet, "/api/thoughts/"+thoughtID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if handlers == nil {
		t.Fatal("handlers = nil")
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got["id"] != thoughtID.String() {
		t.Fatalf("id = %v, want %s", got["id"], thoughtID)
	}
}

func TestListThoughtsSupportsSearchParams(t *testing.T) {
	service := &fakeThoughtService{list: []thoughts.Thought{{ID: uuid.New(), Content: "mcp note", IngestStatus: thoughts.IngestStatusReady}}}
	handlers := NewThoughtHandlers(service)
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodGet, "/api/thoughts?q=mcp&search_mode=keyword&exposure_scope=remote_ok&ingest_status=ready&tag=auth&page=2&page_size=10", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if service.listParams.UserID != user.ID || service.listParams.Q != "mcp" || service.listParams.SearchMode != thoughts.SearchModeKeyword {
		t.Fatalf("list params = %+v", service.listParams)
	}
	if service.listParams.Exposure != "remote_ok" || service.listParams.IngestStatus != "ready" || service.listParams.Tag != "auth" || service.listParams.Page != 2 || service.listParams.PageSize != 10 {
		t.Fatalf("list params = %+v", service.listParams)
	}
}

func TestListThoughtsSemanticIncludesSimilarity(t *testing.T) {
	service := &fakeThoughtService{searchResults: []thoughts.ScoredThought{{
		Thought:    thoughts.Thought{ID: uuid.New(), Content: "career change note", IngestStatus: thoughts.IngestStatusReady},
		Similarity: 0.88,
	}}}
	handlers := NewThoughtHandlers(service)
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodGet, "/api/thoughts?q=career&search_mode=semantic", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if service.searchInput.Query != "career" || service.searchInput.SearchMode != thoughts.SearchModeSemantic {
		t.Fatalf("search input = %+v", service.searchInput)
	}
	var got struct {
		Thoughts []map[string]any `json:"thoughts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Thoughts) != 1 || got.Thoughts[0]["similarity"] != 0.88 {
		t.Fatalf("thoughts = %#v, want similarity in semantic results", got.Thoughts)
	}
}

func TestPatchThoughtReturnsUpdatedJSON(t *testing.T) {
	thoughtID := uuid.New()
	service := &fakeThoughtService{updated: thoughts.Thought{ID: thoughtID, Content: "updated note", ExposureScope: thoughts.ExposureScopeLocalOnly, IngestStatus: thoughts.IngestStatusPending}}
	handlers := NewThoughtHandlers(service)
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodPatch, "/api/thoughts/"+thoughtID.String(), bytes.NewBufferString(`{"content":"updated note","exposure_scope":"local_only","user_tags":["pgx"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if service.updateInput.ThoughtID != thoughtID || service.updateInput.UserID != user.ID {
		t.Fatalf("updateInput = %+v", service.updateInput)
	}
}

func TestDeleteThoughtReturnsNoContent(t *testing.T) {
	thoughtID := uuid.New()
	service := &fakeThoughtService{}
	handlers := NewThoughtHandlers(service)
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodDelete, "/api/thoughts/"+thoughtID.String(), nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if service.deletedThoughtID != thoughtID || service.deletedUserID != user.ID {
		t.Fatalf("deleted IDs = %s/%s", service.deletedUserID, service.deletedThoughtID)
	}
}

func TestRetryThoughtReturnsJSON(t *testing.T) {
	thoughtID := uuid.New()
	service := &fakeThoughtService{retried: thoughts.Thought{ID: thoughtID, Content: "retry me", IngestStatus: thoughts.IngestStatusPending}}
	handlers := NewThoughtHandlers(service)
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodPost, "/api/thoughts/"+thoughtID.String()+"/retry", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.Retry(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if service.retriedThoughtID != thoughtID || service.retriedUserID != user.ID {
		t.Fatalf("retried IDs = %s/%s", service.retriedUserID, service.retriedThoughtID)
	}
}

func TestRelatedThoughtsReturnsScoredJSON(t *testing.T) {
	thoughtID := uuid.New()
	service := &fakeThoughtService{related: []thoughts.ScoredThought{{
		Thought:    thoughts.Thought{ID: uuid.New(), Content: "similar note", IngestStatus: thoughts.IngestStatusReady},
		Similarity: 0.91,
	}}}
	handlers := NewThoughtHandlers(service)
	user := auth.User{ID: uuid.New()}

	req := httptest.NewRequest(http.MethodGet, "/api/thoughts/"+thoughtID.String()+"/related?limit=3", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, user))
	rr := httptest.NewRecorder()

	handlers.Related(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if service.relatedInput.ThoughtID != thoughtID || service.relatedInput.UserID != user.ID || service.relatedInput.Limit != 3 {
		t.Fatalf("related input = %+v", service.relatedInput)
	}
	var got struct {
		Thoughts []map[string]any `json:"thoughts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Thoughts) != 1 || got.Thoughts[0]["similarity"] != 0.91 {
		t.Fatalf("thoughts = %#v, want related similarity", got.Thoughts)
	}
}

type fakeThoughtService struct {
	created          thoughts.Thought
	thought          thoughts.Thought
	list             []thoughts.Thought
	searchResults    []thoughts.ScoredThought
	related          []thoughts.ScoredThought
	updated          thoughts.Thought
	retried          thoughts.Thought
	createInput      thoughts.CreateThoughtInput
	listParams       thoughts.ListThoughtsParams
	searchInput      thoughts.SearchThoughtsInput
	updateInput      thoughts.UpdateThoughtInput
	relatedInput     thoughts.RelatedThoughtsInput
	deletedUserID    uuid.UUID
	deletedThoughtID uuid.UUID
	retriedUserID    uuid.UUID
	retriedThoughtID uuid.UUID
}

func (f *fakeThoughtService) CreateThought(ctx context.Context, input thoughts.CreateThoughtInput) (thoughts.Thought, error) {
	f.createInput = input
	if f.created.ID == uuid.Nil {
		f.created = thoughts.Thought{ID: uuid.New(), Content: input.Content, ExposureScope: input.ExposureScope, UserTags: input.UserTags, IngestStatus: thoughts.IngestStatusPending, CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000000, 0).UTC()}
	}
	return f.created, nil
}

func (f *fakeThoughtService) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	if f.thought.ID == uuid.Nil {
		f.thought = thoughts.Thought{ID: thoughtID, Content: "remember pgx", CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000000, 0).UTC()}
	}
	return f.thought, nil
}

func (f *fakeThoughtService) UpdateThought(ctx context.Context, input thoughts.UpdateThoughtInput) (thoughts.Thought, error) {
	f.updateInput = input
	if f.updated.ID == uuid.Nil {
		f.updated = thoughts.Thought{ID: input.ThoughtID, Content: input.Content, ExposureScope: input.ExposureScope, UserTags: input.UserTags, IngestStatus: thoughts.IngestStatusPending, CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000001, 0).UTC()}
	}
	return f.updated, nil
}

func (f *fakeThoughtService) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	f.deletedUserID = userID
	f.deletedThoughtID = thoughtID
	return nil
}

func (f *fakeThoughtService) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	f.listParams = params
	return f.list, nil
}

func (f *fakeThoughtService) SearchThoughts(ctx context.Context, input thoughts.SearchThoughtsInput) ([]thoughts.ScoredThought, error) {
	f.searchInput = input
	return f.searchResults, nil
}

func (f *fakeThoughtService) RelatedThoughts(ctx context.Context, input thoughts.RelatedThoughtsInput) ([]thoughts.ScoredThought, error) {
	f.relatedInput = input
	return f.related, nil
}

func (f *fakeThoughtService) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	f.retriedUserID = userID
	f.retriedThoughtID = thoughtID
	if f.retried.ID == uuid.Nil {
		f.retried = thoughts.Thought{ID: thoughtID, Content: "retry me", IngestStatus: thoughts.IngestStatusPending, CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000001, 0).UTC()}
	}
	return f.retried, nil
}
