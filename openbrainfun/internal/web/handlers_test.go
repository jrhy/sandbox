package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestIndexPageShowsCaptureFormAndSubmitButton(t *testing.T) {
	userID := uuid.New()
	req := withUser(httptest.NewRequest(http.MethodGet, "/", nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(&fakeThoughtService{}, "test-csrf-key").Index(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="content"`) || !strings.Contains(body, `>Save thought<`) {
		t.Fatalf("body missing capture controls: %s", body)
	}
}

func TestCreateThoughtRedirectsToHome(t *testing.T) {
	userID := uuid.New()
	service := &fakeThoughtService{created: thoughts.Thought{ID: uuid.New(), Content: "Remember MCP auth", CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000000, 0).UTC()}}
	form := url.Values{
		"content":        {"Remember MCP auth"},
		"exposure_scope": {"remote_ok"},
		"user_tags":      {"mcp, auth"},
	}
	req := withUser(httptest.NewRequest(http.MethodPost, "/thoughts", strings.NewReader(form.Encode())), userID)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	NewHandlers(service, "test-csrf-key").Create(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	if location := rr.Header().Get("Location"); !strings.HasPrefix(location, "/?flash=") {
		t.Fatalf("Location = %q, want home flash redirect", location)
	}
	if service.createInput.UserID != userID || service.createInput.ExposureScope != thoughts.ExposureScopeRemoteOK {
		t.Fatalf("createInput = %+v", service.createInput)
	}
	if len(service.createInput.UserTags) != 2 || service.createInput.UserTags[0] != "mcp" || service.createInput.UserTags[1] != "auth" {
		t.Fatalf("UserTags = %#v, want parsed comma-separated tags", service.createInput.UserTags)
	}
}

func TestThoughtPageShowsDeleteButtonAndExtractedMetadata(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	req := withUser(httptest.NewRequest(http.MethodGet, "/thoughts/"+thoughtID.String(), nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(&fakeThoughtService{thought: thoughts.Thought{
		ID:             thoughtID,
		UserID:         userID,
		Content:        "remember mcp auth",
		ExposureScope:  thoughts.ExposureScopeRemoteOK,
		Metadata:       metadata.Metadata{Summary: "remember mcp auth", Topics: []string{"mcp", "auth"}},
		EmbeddingModel: "all-minilm:22m",
	}}, "test-csrf-key").ThoughtDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `>Save changes<`) || !strings.Contains(body, `>Delete thought<`) {
		t.Fatalf("body missing edit controls: %s", body)
	}
	if !strings.Contains(body, "remember mcp auth") || !strings.Contains(body, "all-minilm:22m") {
		t.Fatalf("body missing extracted metadata: %s", body)
	}
}

func TestUpdateThoughtRedirectsToDetail(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	service := &fakeThoughtService{updated: thoughts.Thought{ID: thoughtID, Content: "updated", ExposureScope: thoughts.ExposureScopeLocalOnly, CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000001, 0).UTC()}}
	form := url.Values{
		"content":        {"updated"},
		"exposure_scope": {"local_only"},
		"user_tags":      {"pgx, auth"},
	}
	req := withUser(httptest.NewRequest(http.MethodPost, "/thoughts/"+thoughtID.String(), strings.NewReader(form.Encode())), userID)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	NewHandlers(service, "test-csrf-key").Update(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	if location := rr.Header().Get("Location"); !strings.HasPrefix(location, "/thoughts/"+thoughtID.String()+"?flash=") {
		t.Fatalf("Location = %q, want detail flash redirect", location)
	}
	if service.updateInput.ThoughtID != thoughtID || service.updateInput.UserID != userID {
		t.Fatalf("updateInput = %+v", service.updateInput)
	}
}

func TestThoughtsPageShowsRetryActionForFailedThought(t *testing.T) {
	userID := uuid.New()
	req := withUser(httptest.NewRequest(http.MethodGet, "/thoughts", nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(&fakeThoughtService{list: []thoughts.Thought{{
		ID:           uuid.New(),
		UserID:       userID,
		Content:      "remember retry",
		IngestStatus: thoughts.IngestStatusFailed,
		IngestError:  "ollama down",
	}}}, "test-csrf-key").Thoughts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Retry ingestion") {
		t.Fatalf("body missing retry action: %s", rr.Body.String())
	}
}

func TestThoughtsPageParsesSearchParamsAndShowsSearchControls(t *testing.T) {
	userID := uuid.New()
	service := &fakeThoughtService{list: []thoughts.Thought{{ID: uuid.New(), UserID: userID, Content: "mcp auth", IngestStatus: thoughts.IngestStatusReady}}}
	req := withUser(httptest.NewRequest(http.MethodGet, "/thoughts?q=mcp&search_mode=keyword&exposure_scope=remote_ok&ingest_status=ready&tag=auth&page=2&page_size=10", nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(service, "test-csrf-key").Thoughts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if service.listParams.UserID != userID || service.listParams.Q != "mcp" || service.listParams.SearchMode != thoughts.SearchModeKeyword {
		t.Fatalf("list params = %+v", service.listParams)
	}
	if service.listParams.Exposure != "remote_ok" || service.listParams.IngestStatus != "ready" || service.listParams.Tag != "auth" || service.listParams.Page != 2 || service.listParams.PageSize != 10 {
		t.Fatalf("list params = %+v", service.listParams)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="q"`) || !strings.Contains(body, `name="search_mode"`) {
		t.Fatalf("body missing search controls: %s", body)
	}
}

func TestThoughtsPageShowsSemanticSimilarity(t *testing.T) {
	userID := uuid.New()
	service := &fakeThoughtService{searchResults: []thoughts.ScoredThought{{
		Thought:    thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "career note", IngestStatus: thoughts.IngestStatusReady},
		Similarity: 0.88,
	}}}
	req := withUser(httptest.NewRequest(http.MethodGet, "/thoughts?q=career&search_mode=semantic", nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(service, "test-csrf-key").Thoughts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if service.searchInput.Query != "career" || service.searchInput.SearchMode != thoughts.SearchModeSemantic {
		t.Fatalf("search input = %+v", service.searchInput)
	}
	if !strings.Contains(rr.Body.String(), "Similarity: 88.0%") {
		t.Fatalf("body missing semantic similarity: %s", rr.Body.String())
	}
}

func TestRetryThoughtRedirectsBackToThoughts(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	service := &fakeThoughtService{retried: thoughts.Thought{ID: thoughtID, IngestStatus: thoughts.IngestStatusPending}}
	form := url.Values{"return_to": {"/thoughts"}}
	req := withUser(httptest.NewRequest(http.MethodPost, "/thoughts/"+thoughtID.String()+"/retry", strings.NewReader(form.Encode())), userID)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	NewHandlers(service, "test-csrf-key").Retry(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	if rr.Header().Get("Location") != "/thoughts?flash=Retry+queued" {
		t.Fatalf("Location = %q, want thoughts flash redirect", rr.Header().Get("Location"))
	}
}

func TestDeleteConfirmPageShowsExplicitConfirmation(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	req := withUser(httptest.NewRequest(http.MethodGet, "/thoughts/"+thoughtID.String()+"/delete", nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(&fakeThoughtService{thought: thoughts.Thought{ID: thoughtID, UserID: userID, Content: "remember delete me"}}, "test-csrf-key").DeleteConfirm(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Confirm delete") || !strings.Contains(body, `>Delete thought<`) {
		t.Fatalf("body missing confirmation controls: %s", body)
	}
}

func TestDeleteThoughtRedirectsToThoughts(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	service := &fakeThoughtService{}
	req := withUser(httptest.NewRequest(http.MethodPost, "/thoughts/"+thoughtID.String()+"/delete", nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(service, "test-csrf-key").Delete(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	if rr.Header().Get("Location") != "/thoughts?flash=Thought+deleted" {
		t.Fatalf("Location = %q, want thoughts flash redirect", rr.Header().Get("Location"))
	}
	if service.deletedThoughtID != thoughtID || service.deletedUserID != userID {
		t.Fatalf("deleted IDs = %s/%s", service.deletedUserID, service.deletedThoughtID)
	}
}

func TestThoughtPageShowsRelatedThoughts(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	req := withUser(httptest.NewRequest(http.MethodGet, "/thoughts/"+thoughtID.String(), nil), userID)
	rr := httptest.NewRecorder()

	NewHandlers(&fakeThoughtService{
		thought: thoughts.Thought{
			ID:           thoughtID,
			UserID:       userID,
			Content:      "anchor thought",
			IngestStatus: thoughts.IngestStatusReady,
			Metadata:     metadata.Metadata{Summary: "anchor"},
		},
		related: []thoughts.ScoredThought{{
			Thought:    thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "nearby thought", IngestStatus: thoughts.IngestStatusReady},
			Similarity: 0.91,
		}},
	}, "test-csrf-key").ThoughtDetail(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Related thoughts") || !strings.Contains(body, "Similarity: 91.0%") || !strings.Contains(body, "nearby thought") {
		t.Fatalf("body missing related-thoughts section: %s", body)
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
	err              error
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
	if f.err != nil {
		return thoughts.Thought{}, f.err
	}
	if f.created.ID == uuid.Nil {
		f.created = thoughts.Thought{ID: uuid.New(), Content: input.Content, ExposureScope: input.ExposureScope, UserTags: input.UserTags, IngestStatus: thoughts.IngestStatusPending, CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000000, 0).UTC()}
	}
	return f.created, nil
}

func (f *fakeThoughtService) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	if f.err != nil {
		return thoughts.Thought{}, f.err
	}
	if f.thought.ID == uuid.Nil {
		f.thought = thoughts.Thought{ID: thoughtID, UserID: userID, Content: "remember pgx", CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000000, 0).UTC()}
	}
	return f.thought, nil
}

func (f *fakeThoughtService) UpdateThought(ctx context.Context, input thoughts.UpdateThoughtInput) (thoughts.Thought, error) {
	f.updateInput = input
	if f.err != nil {
		return thoughts.Thought{}, f.err
	}
	if f.updated.ID == uuid.Nil {
		f.updated = thoughts.Thought{ID: input.ThoughtID, Content: input.Content, ExposureScope: input.ExposureScope, UserTags: input.UserTags, IngestStatus: thoughts.IngestStatusPending, CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000001, 0).UTC()}
	}
	return f.updated, nil
}

func (f *fakeThoughtService) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.deletedUserID = userID
	f.deletedThoughtID = thoughtID
	return nil
}

func (f *fakeThoughtService) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	f.listParams = params
	if f.err != nil {
		return nil, f.err
	}
	return append([]thoughts.Thought(nil), f.list...), nil
}

func (f *fakeThoughtService) SearchThoughts(ctx context.Context, input thoughts.SearchThoughtsInput) ([]thoughts.ScoredThought, error) {
	f.searchInput = input
	if f.err != nil {
		return nil, f.err
	}
	return append([]thoughts.ScoredThought(nil), f.searchResults...), nil
}

func (f *fakeThoughtService) RelatedThoughts(ctx context.Context, input thoughts.RelatedThoughtsInput) ([]thoughts.ScoredThought, error) {
	f.relatedInput = input
	if f.err != nil {
		return nil, f.err
	}
	return append([]thoughts.ScoredThought(nil), f.related...), nil
}

func (f *fakeThoughtService) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	if f.err != nil {
		return thoughts.Thought{}, f.err
	}
	f.retriedUserID = userID
	f.retriedThoughtID = thoughtID
	if f.retried.ID == uuid.Nil {
		f.retried = thoughts.Thought{ID: thoughtID, Content: "retry me", IngestStatus: thoughts.IngestStatusPending, CreatedAt: time.Unix(1700000000, 0).UTC(), UpdatedAt: time.Unix(1700000001, 0).UTC()}
	}
	return f.retried, nil
}

func withUser(req *http.Request, userID uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), auth.ContextUserKey{}, auth.User{ID: userID, Username: "alice"})
	ctx = context.WithValue(ctx, auth.ContextSessionTokenKey{}, "session-token")
	return req.WithContext(ctx)
}
