package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/app"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/config"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/ollama"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultBackend       = "fake"
	defaultSessionTTL    = 24 * time.Hour
	defaultDemoPassword  = "demo-password"
	defaultOtherPassword = "other-password"
	defaultDemoToken     = "demo-mcp-token"
	defaultOtherToken    = "other-mcp-token"
)

type TestEnv struct {
	webBaseURL string
	mcpBaseURL string
	runtime    app.Runtime

	demoUser   auth.User
	otherUser  auth.User
	demoToken  string
	otherToken string
}

type AuthenticatedClient struct {
	HTTPClient *http.Client
	CSRFToken  string
}

type CreateThoughtRequest struct {
	Content       string   `json:"content"`
	ExposureScope string   `json:"exposure_scope"`
	UserTags      []string `json:"user_tags,omitempty"`
}

type UpdateThoughtRequest struct {
	Content       string   `json:"content"`
	ExposureScope string   `json:"exposure_scope"`
	UserTags      []string `json:"user_tags,omitempty"`
}

type ThoughtResponse struct {
	ID            string            `json:"id"`
	Content       string            `json:"content"`
	ExposureScope string            `json:"exposure_scope"`
	UserTags      []string          `json:"user_tags"`
	Metadata      metadata.Metadata `json:"metadata"`
	Similarity    *float64          `json:"similarity,omitempty"`
	IngestStatus  string            `json:"ingest_status"`
	IngestError   string            `json:"ingest_error,omitempty"`
}

func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	return newTestEnvWithConfig(t, config.Config{
		CookieSecure: false,
		CSRFKey:      "test-csrf-key",
		SessionTTL:   defaultSessionTTL,
	})
}

func newTestEnvWithConfig(t *testing.T, cfg config.Config) *TestEnv {
	t.Helper()

	passwordHash, err := auth.HashPassword(defaultDemoPassword)
	if err != nil {
		t.Fatalf("HashPassword(demo) error = %v", err)
	}
	otherPasswordHash, err := auth.HashPassword(defaultOtherPassword)
	if err != nil {
		t.Fatalf("HashPassword(other) error = %v", err)
	}

	demoUser := auth.User{ID: uuid.New(), Username: "demo", PasswordHash: passwordHash}
	otherUser := auth.User{ID: uuid.New(), Username: "other", PasswordHash: otherPasswordHash}
	authRepo := newMemoryAuthRepo(demoUser, otherUser, defaultDemoToken, defaultOtherToken)
	thoughtRepo := newMemoryThoughtRepo()
	embedder, extractor := buildProviders(t)
	if cfg.CSRFKey == "" {
		cfg.CSRFKey = "test-csrf-key"
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = defaultSessionTTL
	}

	runtime := app.Build(cfg, authRepo, thoughtRepo, embedder, extractor)
	env := &TestEnv{
		webBaseURL: "http://openbrain-web.test",
		mcpBaseURL: "http://openbrain-mcp.test",
		runtime:    runtime,
		demoUser:   demoUser,
		otherUser:  otherUser,
		demoToken:  defaultDemoToken,
		otherToken: defaultOtherToken,
	}
	return env
}

func buildProviders(t *testing.T) (embed.Embedder, metadata.Extractor) {
	t.Helper()

	ollamaURL := strings.TrimSpace(os.Getenv("OPENBRAIN_OLLAMA_URL"))
	if ollamaURL == "" {
		ollamaURL = "http://127.0.0.1:11434"
	}
	client := ollama.NewClient(ollamaURL, nil)

	var embedder embed.Embedder
	switch strings.TrimSpace(os.Getenv("OPENBRAIN_EMBED_BACKEND")) {
	case "", defaultBackend:
		embedder = embed.NewFake(map[string][]float32{
			"Remember MCP auth":                               {1, 0, 0},
			"Remember MCP auth and sessions":                  {1, 1, 0},
			"Remember MCP auth for Open WebUI local sessions": {1, 1, 1},
		})
	case "ollama":
		embedder = ollama.NewProvider(client, requiredEnv(t, "OPENBRAIN_EMBED_MODEL"))
	default:
		t.Fatalf("unsupported OPENBRAIN_EMBED_BACKEND %q", os.Getenv("OPENBRAIN_EMBED_BACKEND"))
	}

	var extractor metadata.Extractor
	switch strings.TrimSpace(os.Getenv("OPENBRAIN_METADATA_BACKEND")) {
	case "", defaultBackend:
		extractor = metadata.NewFake(map[string]metadata.Metadata{
			"Remember MCP auth": {
				Summary:  "Remember MCP auth.",
				Topics:   []string{"MCP", "auth"},
				Entities: []string{"demo"},
			},
			"Remember MCP auth and sessions": {
				Summary:  "Remember MCP auth and session handling.",
				Topics:   []string{"MCP", "auth", "sessions"},
				Entities: []string{"demo"},
			},
			"Remember MCP auth for Open WebUI local sessions": {
				Summary:  "Remember MCP auth for Open WebUI local sessions.",
				Topics:   []string{"MCP", "Open WebUI", "sessions"},
				Entities: []string{"Open WebUI"},
			},
		})
	case "ollama":
		extractor = ollama.NewMetadataProvider(client, requiredEnv(t, "OPENBRAIN_METADATA_MODEL"))
	default:
		t.Fatalf("unsupported OPENBRAIN_METADATA_BACKEND %q", os.Getenv("OPENBRAIN_METADATA_BACKEND"))
	}
	return embedder, extractor
}

func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required when OPENBRAIN_EMBED_BACKEND=ollama", name)
	}
	return value
}

func (e *TestEnv) LoginAsDemoUser(t *testing.T) AuthenticatedClient {
	t.Helper()
	return e.login(t, e.demoUser.Username, defaultDemoPassword)
}

func (e *TestEnv) DemoToken() string {
	return e.demoToken
}

func (e *TestEnv) CreateThought(t *testing.T, client AuthenticatedClient, req CreateThoughtRequest) ThoughtResponse {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, e.webBaseURL+"/api/thoughts", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-CSRF-Token", client.CSRFToken)

	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		t.Fatalf("POST /api/thoughts error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/thoughts status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var thought ThoughtResponse
	if err := json.NewDecoder(resp.Body).Decode(&thought); err != nil {
		t.Fatalf("decode thought response: %v", err)
	}

	e.processPending(t)
	return thought
}

func (e *TestEnv) GetThought(t *testing.T, client AuthenticatedClient, thoughtID string) ThoughtResponse {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodGet, e.webBaseURL+"/api/thoughts/"+thoughtID, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		t.Fatalf("GET /api/thoughts/%s error = %v", thoughtID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/thoughts/%s status = %d, want %d", thoughtID, resp.StatusCode, http.StatusOK)
	}

	var thought ThoughtResponse
	if err := json.NewDecoder(resp.Body).Decode(&thought); err != nil {
		t.Fatalf("decode thought response: %v", err)
	}
	return thought
}

func (e *TestEnv) UpdateThought(t *testing.T, client AuthenticatedClient, thoughtID string, req UpdateThoughtRequest) ThoughtResponse {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	httpReq, err := http.NewRequest(http.MethodPatch, e.webBaseURL+"/api/thoughts/"+thoughtID, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-CSRF-Token", client.CSRFToken)

	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		t.Fatalf("PATCH /api/thoughts/%s error = %v", thoughtID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/thoughts/%s status = %d, want %d", thoughtID, resp.StatusCode, http.StatusOK)
	}

	var thought ThoughtResponse
	if err := json.NewDecoder(resp.Body).Decode(&thought); err != nil {
		t.Fatalf("decode thought response: %v", err)
	}
	return thought
}

func (e *TestEnv) SearchThoughts(t *testing.T, client AuthenticatedClient, query string) []ThoughtResponse {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodGet, e.webBaseURL+"/api/thoughts?q="+url.QueryEscape(query)+"&search_mode=semantic", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		t.Fatalf("GET /api/thoughts semantic search error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/thoughts semantic search status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var payload struct {
		Thoughts []ThoughtResponse `json:"thoughts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	return payload.Thoughts
}

func (e *TestEnv) RelatedThoughts(t *testing.T, client AuthenticatedClient, thoughtID string) []ThoughtResponse {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodGet, e.webBaseURL+"/api/thoughts/"+thoughtID+"/related?limit=5", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		t.Fatalf("GET /api/thoughts/%s/related error = %v", thoughtID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/thoughts/%s/related status = %d, want %d", thoughtID, resp.StatusCode, http.StatusOK)
	}

	var payload struct {
		Thoughts []ThoughtResponse `json:"thoughts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode related response: %v", err)
	}
	return payload.Thoughts
}

func (e *TestEnv) DeleteThought(t *testing.T, client AuthenticatedClient, thoughtID string) {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodDelete, e.webBaseURL+"/api/thoughts/"+thoughtID, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	httpReq.Header.Set("X-CSRF-Token", client.CSRFToken)

	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		t.Fatalf("DELETE /api/thoughts/%s error = %v", thoughtID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/thoughts/%s status = %d, want %d", thoughtID, resp.StatusCode, http.StatusNoContent)
	}
}

func (e *TestEnv) AssertThoughtNotFound(t *testing.T, client AuthenticatedClient, thoughtID string) {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodGet, e.webBaseURL+"/api/thoughts/"+thoughtID, nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	resp, err := client.HTTPClient.Do(httpReq)
	if err != nil {
		t.Fatalf("GET /api/thoughts/%s error = %v", thoughtID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/thoughts/%s status = %d, want %d", thoughtID, resp.StatusCode, http.StatusNotFound)
	}
}

func (e *TestEnv) AssertMCPFindsThought(t *testing.T, token, query string) {
	t.Helper()

	session := e.mcpSession(t, token)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_thoughts",
		Arguments: map[string]any{"query": query},
	})
	if err != nil {
		t.Fatalf("CallTool(search_thoughts) error = %v", err)
	}
	if !toolResultContains(result, query) {
		t.Fatalf("search_thoughts result does not contain %q: %+v", query, result)
	}
}

func (e *TestEnv) AssertOtherUsersMCPTokenCannotSeeThought(t *testing.T, thoughtID string) {
	t.Helper()

	session := e.mcpSession(t, e.otherToken)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_thought",
		Arguments: map[string]any{"id": thoughtID},
	})
	if err != nil {
		t.Fatalf("CallTool(get_thought) transport error = %v", err)
	}
	if !result.IsError || !toolResultContains(result, "thought not found") {
		t.Fatalf("CallTool(get_thought) = %+v, want tool-level not found error", result)
	}
}

func (e *TestEnv) AssertMCPRelatedThoughts(t *testing.T, token, thoughtID, want string) {
	t.Helper()

	session := e.mcpSession(t, token)
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "related_thoughts",
		Arguments: map[string]any{"id": thoughtID},
	})
	if err != nil {
		t.Fatalf("CallTool(related_thoughts) error = %v", err)
	}
	if !toolResultContains(result, want) {
		t.Fatalf("related_thoughts result does not contain %q: %+v", want, result)
	}
}

func (e *TestEnv) login(t *testing.T, username, password string) AuthenticatedClient {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{
		Jar:       jar,
		Transport: handlerRoundTripper{handler: e.runtime.WebHandler},
	}
	body := bytes.NewBufferString(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password))
	resp, err := client.Post(e.webBaseURL+"/api/session", "application/json", body)
	if err != nil {
		t.Fatalf("POST /api/session error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var payload struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if payload.CSRFToken == "" {
		t.Fatal("csrf_token = empty, want value")
	}

	return AuthenticatedClient{HTTPClient: client, CSRFToken: payload.CSRFToken}
}

func (e *TestEnv) processPending(t *testing.T) {
	t.Helper()
	if err := e.runtime.Processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("Processor.RunOnce() error = %v", err)
	}
}

func (e *TestEnv) ProcessPending(t *testing.T) {
	t.Helper()
	e.processPending(t)
}

func (e *TestEnv) mcpSession(t *testing.T, token string) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-client", Version: "v0.0.1"}, nil)
	httpClient := &http.Client{Transport: bearerRoundTripper{token: token, base: handlerRoundTripper{handler: e.runtime.MCPHandler}}}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   e.mcpBaseURL + "/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	return session
}

func toolResultContains(result *mcp.CallToolResult, want string) bool {
	for _, item := range result.Content {
		text, ok := item.(*mcp.TextContent)
		if ok && strings.Contains(text.Text, want) {
			return true
		}
	}
	return false
}

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (r bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header.Set("Authorization", "Bearer "+r.token)
	base := r.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

type handlerRoundTripper struct {
	handler http.Handler
}

func (r handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	r.handler.ServeHTTP(recorder, req.Clone(req.Context()))
	response := recorder.Result()
	response.Request = req
	return response, nil
}

type memoryAuthRepo struct {
	mu            sync.Mutex
	usersByName   map[string]auth.User
	sessionsByKey map[string]auth.Session
	usersByID     map[uuid.UUID]auth.User
	mcpTokens     map[string]auth.MCPToken
}

func newMemoryAuthRepo(users ...any) *memoryAuthRepo {
	repo := &memoryAuthRepo{
		usersByName:   make(map[string]auth.User),
		sessionsByKey: make(map[string]auth.Session),
		usersByID:     make(map[uuid.UUID]auth.User),
		mcpTokens:     make(map[string]auth.MCPToken),
	}
	if len(users) != 4 {
		panic("newMemoryAuthRepo expects demoUser, otherUser, demoToken, otherToken")
	}
	demoUser := users[0].(auth.User)
	otherUser := users[1].(auth.User)
	demoToken := users[2].(string)
	otherToken := users[3].(string)
	repo.usersByName[demoUser.Username] = demoUser
	repo.usersByName[otherUser.Username] = otherUser
	repo.usersByID[demoUser.ID] = demoUser
	repo.usersByID[otherUser.ID] = otherUser
	now := time.Now().UTC()
	repo.mcpTokens[auth.HashToken(demoToken)] = auth.MCPToken{ID: uuid.New(), UserID: demoUser.ID, TokenHash: auth.HashToken(demoToken), Label: "demo", CreatedAt: now}
	repo.mcpTokens[auth.HashToken(otherToken)] = auth.MCPToken{ID: uuid.New(), UserID: otherUser.ID, TokenHash: auth.HashToken(otherToken), Label: "other", CreatedAt: now}
	return repo
}

func (r *memoryAuthRepo) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.usersByName[username]
	if !ok {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}

func (r *memoryAuthRepo) CreateSession(ctx context.Context, params auth.CreateSessionParams) (auth.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := auth.Session{
		ID:               params.ID,
		UserID:           params.UserID,
		SessionTokenHash: params.SessionTokenHash,
		ExpiresAt:        params.ExpiresAt,
		CreatedAt:        params.CreatedAt,
		LastSeenAt:       params.LastSeenAt,
	}
	r.sessionsByKey[params.SessionTokenHash] = session
	return session, nil
}

func (r *memoryAuthRepo) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, session := range r.sessionsByKey {
		if session.ID == sessionID {
			delete(r.sessionsByKey, key)
			return nil
		}
	}
	return nil
}

func (r *memoryAuthRepo) FindSessionByTokenHash(ctx context.Context, tokenHash string) (auth.Session, auth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessionsByKey[tokenHash]
	if !ok {
		return auth.Session{}, auth.User{}, auth.ErrUserNotFound
	}
	user, ok := r.usersByID[session.UserID]
	if !ok {
		return auth.Session{}, auth.User{}, auth.ErrUserNotFound
	}
	return session, user, nil
}

func (r *memoryAuthRepo) FindUserByMCPTokenHash(ctx context.Context, tokenHash string) (auth.User, auth.MCPToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.mcpTokens[tokenHash]
	if !ok {
		return auth.User{}, auth.MCPToken{}, auth.ErrUserNotFound
	}
	user, ok := r.usersByID[token.UserID]
	if !ok {
		return auth.User{}, auth.MCPToken{}, auth.ErrUserNotFound
	}
	return user, token, nil
}

func (r *memoryAuthRepo) TouchSessionActivity(ctx context.Context, sessionID uuid.UUID, seenAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, session := range r.sessionsByKey {
		if session.ID != sessionID {
			continue
		}
		session.LastSeenAt = seenAt
		r.sessionsByKey[key] = session
		return nil
	}
	return nil
}

func (r *memoryAuthRepo) TouchMCPTokenActivity(ctx context.Context, tokenID uuid.UUID, usedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, token := range r.mcpTokens {
		if token.ID != tokenID {
			continue
		}
		token.LastUsedAt = usedAt
		r.mcpTokens[key] = token
		return nil
	}
	return nil
}

type memoryThoughtRepo struct {
	mu       sync.Mutex
	thoughts map[uuid.UUID]thoughts.Thought
}

func newMemoryThoughtRepo() *memoryThoughtRepo {
	return &memoryThoughtRepo{thoughts: make(map[uuid.UUID]thoughts.Thought)}
}

func (r *memoryThoughtRepo) CreateThought(ctx context.Context, thought thoughts.Thought) (thoughts.Thought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.thoughts[thought.ID] = cloneThought(thought)
	return cloneThought(thought), nil
}

func (r *memoryThoughtRepo) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	thought, ok := r.thoughts[thoughtID]
	if !ok || thought.UserID != userID {
		return thoughts.Thought{}, thoughts.ErrThoughtNotFound
	}
	return cloneThought(thought), nil
}

func (r *memoryThoughtRepo) UpdateThought(ctx context.Context, params thoughts.UpdateThoughtParams) (thoughts.Thought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.thoughts[params.ThoughtID]
	if !ok || current.UserID != params.UserID {
		return thoughts.Thought{}, thoughts.ErrThoughtNotFound
	}
	current.Content = params.Content
	current.ExposureScope = params.ExposureScope
	current.UserTags = append([]string(nil), params.UserTags...)
	current.IngestStatus = params.IngestStatus
	current.IngestError = params.IngestError
	current.UpdatedAt = params.UpdatedAt
	r.thoughts[current.ID] = cloneThought(current)
	return cloneThought(current), nil
}

func (r *memoryThoughtRepo) DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.thoughts[thoughtID]
	if !ok || current.UserID != userID {
		return thoughts.ErrThoughtNotFound
	}
	delete(r.thoughts, thoughtID)
	return nil
}

func (r *memoryThoughtRepo) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return filterAndSortThoughts(r.thoughts, params), nil
}

func (r *memoryThoughtRepo) SearchKeyword(ctx context.Context, params thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	listParams := thoughts.ListThoughtsParams{
		UserID:       params.UserID,
		Q:            params.Query,
		Exposure:     params.Exposure,
		IngestStatus: params.IngestStatus,
		Tag:          params.Tag,
		Page:         params.Page,
		PageSize:     params.PageSize,
	}
	return filterAndSortThoughts(r.thoughts, listParams), nil
}

func (r *memoryThoughtRepo) SearchSemantic(ctx context.Context, params thoughts.SearchSemanticParams) ([]thoughts.ScoredThought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]thoughts.ScoredThought, 0)
	for _, thought := range r.thoughts {
		if thought.UserID != params.UserID || thought.IngestStatus != thoughts.IngestStatusReady || len(thought.Embedding) == 0 {
			continue
		}
		if params.Exposure != "" && string(thought.ExposureScope) != params.Exposure {
			continue
		}
		if params.Tag != "" && !containsTag(thought.UserTags, params.Tag) {
			continue
		}
		similarity := cosineSimilarity(thought.Embedding, params.QueryEmbedding)
		if params.Threshold > 0 && similarity <= params.Threshold {
			continue
		}
		items = append(items, thoughts.ScoredThought{Thought: cloneThought(thought), Similarity: similarity})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Similarity == items[j].Similarity {
			return items[i].Thought.ID.String() > items[j].Thought.ID.String()
		}
		return items[i].Similarity > items[j].Similarity
	})
	if len(items) > 0 {
		start := semanticOffset(params.Page, params.PageSize)
		if start >= len(items) {
			return []thoughts.ScoredThought{}, nil
		}
		end := start + semanticPageSize(params.PageSize)
		if end > len(items) {
			end = len(items)
		}
		items = items[start:end]
	}
	return items, nil
}

func (r *memoryThoughtRepo) RelatedThoughts(ctx context.Context, params thoughts.RelatedThoughtsParams) ([]thoughts.ScoredThought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	anchor, ok := r.thoughts[params.ThoughtID]
	if !ok || anchor.UserID != params.UserID {
		return nil, thoughts.ErrThoughtNotFound
	}
	if anchor.IngestStatus != thoughts.IngestStatusReady || len(anchor.Embedding) == 0 {
		return []thoughts.ScoredThought{}, nil
	}
	items := make([]thoughts.ScoredThought, 0)
	for _, thought := range r.thoughts {
		if thought.UserID != params.UserID || thought.ID == params.ThoughtID || thought.IngestStatus != thoughts.IngestStatusReady || len(thought.Embedding) == 0 {
			continue
		}
		if params.Exposure != "" && string(thought.ExposureScope) != params.Exposure {
			continue
		}
		items = append(items, thoughts.ScoredThought{
			Thought:    cloneThought(thought),
			Similarity: cosineSimilarity(anchor.Embedding, thought.Embedding),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Similarity == items[j].Similarity {
			return items[i].Thought.ID.String() > items[j].Thought.ID.String()
		}
		return items[i].Similarity > items[j].Similarity
	})
	if limit := semanticPageSize(params.Limit); len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *memoryThoughtRepo) RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.thoughts[thoughtID]
	if !ok || current.UserID != userID {
		return thoughts.Thought{}, thoughts.ErrThoughtNotFound
	}
	current.IngestStatus = thoughts.IngestStatusPending
	current.IngestError = ""
	current.UpdatedAt = time.Now().UTC()
	r.thoughts[current.ID] = cloneThought(current)
	return cloneThought(current), nil
}

func (r *memoryThoughtRepo) ClaimPending(ctx context.Context, limit int) ([]thoughts.Thought, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]thoughts.Thought, 0)
	for _, thought := range r.thoughts {
		if thought.EmbeddingStatus == thoughts.IngestStatusPending || thought.MetadataStatus == thoughts.IngestStatusPending {
			items = append(items, cloneThought(thought))
		}
	}
	sortThoughts(items)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *memoryThoughtRepo) MarkProcessed(ctx context.Context, params thoughts.MarkProcessedParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.thoughts[params.ThoughtID]
	if !ok {
		return thoughts.ErrThoughtNotFound
	}
	if params.UpdateEmbedding {
		current.Embedding = append([]float32(nil), params.Embedding...)
		current.EmbeddingModel = params.EmbeddingModel
		current.EmbeddingFingerprint = params.EmbeddingFingerprint
		current.EmbeddingStatus = params.EmbeddingStatus
		current.EmbeddingError = params.EmbeddingError
	}
	if params.UpdateMetadata {
		current.Metadata = params.Metadata
		current.MetadataModel = params.MetadataModel
		current.MetadataFingerprint = params.MetadataFingerprint
		current.MetadataStatus = params.MetadataStatus
		current.MetadataError = params.MetadataError
	}
	current.IngestStatus = params.IngestStatus
	current.IngestError = params.IngestError
	current.UpdatedAt = params.ProcessedAt
	r.thoughts[current.ID] = cloneThought(current)
	return nil
}

func (r *memoryThoughtRepo) MarkEmbeddingFailed(ctx context.Context, params thoughts.MarkEmbeddingFailedParams) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.thoughts[params.ThoughtID]
	if !ok {
		return thoughts.ErrThoughtNotFound
	}
	current.EmbeddingStatus = thoughts.IngestStatusFailed
	current.EmbeddingError = params.Reason
	current.IngestStatus = thoughts.IngestStatusFailed
	current.IngestError = params.Reason
	current.UpdatedAt = params.FailedAt
	r.thoughts[current.ID] = cloneThought(current)
	return nil
}

func (r *memoryThoughtRepo) ReconcileModels(ctx context.Context, params thoughts.ReconcileModelsParams) (thoughts.ReconcileModelsResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result thoughts.ReconcileModelsResult
	for id, thought := range r.thoughts {
		if thought.EmbeddingModel != params.EmbeddingModel || thought.EmbeddingFingerprint != params.EmbeddingFingerprint {
			thought.EmbeddingStatus = thoughts.IngestStatusPending
			thought.EmbeddingError = ""
			thought.IngestStatus = thoughts.IngestStatusPending
			thought.IngestError = ""
			result.EmbeddingMarked++
		}
		if thought.MetadataModel != params.MetadataModel || thought.MetadataFingerprint != params.MetadataFingerprint {
			thought.MetadataStatus = thoughts.IngestStatusPending
			thought.MetadataError = ""
			if thought.IngestStatus != thoughts.IngestStatusFailed {
				thought.IngestStatus = thoughts.IngestStatusPending
				thought.IngestError = ""
			}
			result.MetadataMarked++
		}
		thought.UpdatedAt = params.ReconciledAt
		r.thoughts[id] = cloneThought(thought)
	}
	return result, nil
}

func filterAndSortThoughts(items map[uuid.UUID]thoughts.Thought, params thoughts.ListThoughtsParams) []thoughts.Thought {
	filtered := make([]thoughts.Thought, 0)
	query := strings.ToLower(strings.TrimSpace(params.Q))
	for _, item := range items {
		if item.UserID != params.UserID {
			continue
		}
		if params.Exposure != "" && string(item.ExposureScope) != params.Exposure {
			continue
		}
		if params.IngestStatus != "" && string(item.IngestStatus) != params.IngestStatus {
			continue
		}
		if params.Tag != "" && !containsTag(item.UserTags, params.Tag) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(item.Content), query) {
			continue
		}
		filtered = append(filtered, cloneThought(item))
	}
	sortThoughts(filtered)
	return paginate(filtered, params.Page, params.PageSize)
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func cosineSimilarity(left, right []float32) float64 {
	var dot, leftNorm, rightNorm float64
	for i := 0; i < len(left) && i < len(right); i++ {
		l := float64(left[i])
		r := float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func semanticPageSize(pageSize int) int {
	if pageSize <= 0 {
		return 20
	}
	return pageSize
}

func semanticOffset(page, pageSize int) int {
	if page <= 1 {
		return 0
	}
	return (page - 1) * semanticPageSize(pageSize)
}

func paginate(items []thoughts.Thought, page, pageSize int) []thoughts.Thought {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 1 {
		page = 1
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []thoughts.Thought{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return append([]thoughts.Thought(nil), items[start:end]...)
}

func sortThoughts(items []thoughts.Thought) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID.String() > items[j].ID.String()
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

func cloneThought(thought thoughts.Thought) thoughts.Thought {
	cloned := thought
	cloned.UserTags = append([]string(nil), thought.UserTags...)
	cloned.Embedding = append([]float32(nil), thought.Embedding...)
	cloned.Metadata = metadata.Normalize(map[string]any{
		"summary":  thought.Metadata.Summary,
		"topics":   append([]string(nil), thought.Metadata.Topics...),
		"entities": append([]string(nil), thought.Metadata.Entities...),
	})
	return cloned
}
