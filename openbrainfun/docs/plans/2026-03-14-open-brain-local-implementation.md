# Local Open Brain Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a multi-user local-first open-brain service with DB-backed login sessions, per-user thought ownership, per-user MCP token access on a separate port, v1 metadata extraction, a generated README walkthrough, and CI that verifies both fake and real Ollama embedding backends.

**Architecture:** One Go binary serves the browser UI and JSON API on the web listener and the read-only MCP server on a second listener. Postgres stores users, sessions, MCP tokens, thoughts, ingest state, and normalized extracted metadata. Ollama supplies embeddings and metadata extraction behind small interfaces so fake and real providers can run through the same contract/integration suites.

**Tech Stack:** Go, `net/http`, `html/template`, `github.com/jackc/pgx/v5`, `github.com/modelcontextprotocol/go-sdk`, Postgres, pgvector, pgcrypto, Ollama, Docker Compose, GitHub Actions.

---

## Implementation notes before starting

- In this monorepo, treat `openbrainfun/` as the project root. Keep
  application code under this directory (`cmd/`, `internal/`, `migrations/`,
  `scripts/`, `README.md`) and do not modify the sandbox repo root.
- For Task 1, a minimal nested `go.mod` and empty package directories are
  acceptable test harness setup before the first red test. Do not implement
  application behavior before watching the first test fail.
- The README walkthrough is a deliverable, not a follow-up. Design the code so the walkthrough can be generated from real interactions.
- Use `all-minilm:22m` for the README quickstart and the real-provider CI job. Allow `OPENBRAIN_EMBED_MODEL` to override it.
- Add a separate `OPENBRAIN_METADATA_MODEL` for the metadata-extraction model; do not tie it to vector dimensions.
- Do not hardcode embedding dimension blindly. Probe the configured Ollama model and render the migration with the probed dimension before applying it.
- The web listener and MCP listener must be separate addresses from day one.
- The same contract/integration suites must be runnable with `OPENBRAIN_EMBED_BACKEND=fake` and `OPENBRAIN_EMBED_BACKEND=ollama`.
- Manual tags remain first-class. Extracted metadata is read-only and normalized before persistence.
- MCP stays read-only in v1, but the thought creation service should stay transport-agnostic so a future `capture_thought` MCP tool can reuse it.
- Do not leave the worker loop implicit: Task 7 must wire and test the polling loop from startup, not just the `RunOnce` processor logic.
- Do not leave the app on noop wiring: Task 7 must also compose the real auth/thought stores, services, and web/API handlers into startup so Task 11/12 are not the first place integration fails.

### Task 1: Bootstrap the module, config, and dual-listener server skeleton

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `cmd/openbrain/main.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`
- Create: `.gitignore`

**Step 1: Write the failing config test**

```go
func TestLoadFromEnv(t *testing.T) {
	t.Setenv("OPENBRAIN_WEB_ADDR", "127.0.0.1:8080")
	t.Setenv("OPENBRAIN_MCP_ADDR", "127.0.0.1:8081")
	t.Setenv("OPENBRAIN_DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/openbrain?sslmode=disable")
	t.Setenv("OPENBRAIN_OLLAMA_URL", "http://127.0.0.1:11434")
	t.Setenv("OPENBRAIN_EMBED_MODEL", "all-minilm:22m")
	t.Setenv("OPENBRAIN_METADATA_MODEL", "small-metadata-model")
	t.Setenv("OPENBRAIN_SESSION_TTL", "24h")
	t.Setenv("OPENBRAIN_COOKIE_SECURE", "false")
	t.Setenv("OPENBRAIN_CSRF_KEY", "test-csrf-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WebAddr != "127.0.0.1:8080" || cfg.MCPAddr != "127.0.0.1:8081" {
		t.Fatalf("unexpected addrs: %+v", cfg)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestLoadFromEnv -v`
Expected: FAIL because the module and `Load` do not exist yet.

**Step 3: Write minimal implementation**

- Initialize the module.
- Add `Config` with web/MCP addresses, database URL, Ollama URL, embed model, metadata model, session TTL, cookie-security mode, CSRF key, and log level.
- Add `Load()` with validation and explicit duration parsing.
- Add a small `server.Server` that can build empty web and MCP `http.Server` values plus `/healthz` on the web listener.
- Add `.gitignore` entries for `var/`, `.cover/`, and local binaries.

```go
type Config struct {
	WebAddr      string
	MCPAddr      string
	DatabaseURL  string
	OllamaURL    string
	EmbedModel   string
	MetadataModel string
	SessionTTL   time.Duration
	CookieSecure bool
	CSRFKey      string
	LogLevel     string
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config ./internal/server -v`
Expected: PASS

**Step 5: Commit**

```bash
git add go.mod go.sum cmd/openbrain/main.go internal/config/config.go internal/config/config_test.go internal/server/server.go internal/server/server_test.go .gitignore
git commit -m "openbrainfun: bootstrap module and config"
```

### Task 2: Add migration rendering, schema templates, and core domain types

**Files:**
- Create: `migrations/0001_initial.sql.tmpl`
- Create: `internal/migrations/render.go`
- Create: `internal/migrations/render_test.go`
- Create: `internal/auth/types.go`
- Create: `internal/auth/types_test.go`
- Create: `internal/metadata/types.go`
- Create: `internal/metadata/types_test.go`
- Create: `internal/thoughts/types.go`
- Create: `internal/thoughts/types_test.go`
- Create: `scripts/render-migration.sh`

**Step 1: Write the failing migration render test**

```go
func TestRenderInitialSchemaReplacesEmbedDimensions(t *testing.T) {
	tmpl := "create table thoughts (embedding vector(__EMBED_DIMENSIONS__));"
	got, err := RenderSchema(tmpl, 384)
	if err != nil {
		t.Fatalf("RenderSchema() error = %v", err)
	}
	if !strings.Contains(got, "vector(384)") {
		t.Fatalf("got %q, want rendered dimension", got)
	}
}
```

```go
func TestNormalizeProvidesSafeDefaults(t *testing.T) {
	got := metadata.Normalize(map[string]any{"topics": "not-an-array"})
	if got.Summary == "" {
		t.Fatal("expected non-empty summary default")
	}
	if got.Topics == nil {
		t.Fatal("expected topics slice to be initialized")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/migrations ./internal/auth ./internal/metadata ./internal/thoughts -run 'TestRenderInitialSchemaReplacesEmbedDimensions|TestNormalizeProvidesSafeDefaults|TestNewThoughtRejectsBlankContent' -v`
Expected: FAIL because the render function and domain types do not exist.

**Step 3: Write minimal implementation**

- Add the SQL template containing `users`, `web_sessions`, `mcp_tokens`, and `thoughts`.
- Add `RenderSchema(template string, embedDimensions int)`.
- Add normalized metadata types plus `Normalize(raw map[string]any) Metadata`.
- Add `User`, `Session`, `MCPToken`, `Thought`, `ExposureScope`, and `IngestStatus` domain types.
- Add `NewThought(params)` validation and sentinel errors for blank content / invalid scope.
- Add `scripts/render-migration.sh` that calls a tiny Go helper or `go test`-verified package to render `migrations/0001_initial.sql` from the template.

```go
var (
	ErrBlankContent      = errors.New("blank content")
	ErrInvalidExposure   = errors.New("invalid exposure scope")
	ErrInvalidUsername   = errors.New("invalid username")
)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/migrations ./internal/auth ./internal/metadata ./internal/thoughts -v`
Expected: PASS

**Step 5: Commit**

```bash
git add migrations/0001_initial.sql.tmpl internal/migrations/render.go internal/migrations/render_test.go internal/auth/types.go internal/auth/types_test.go internal/metadata/types.go internal/metadata/types_test.go internal/thoughts/types.go internal/thoughts/types_test.go scripts/render-migration.sh
git commit -m "openbrainfun: add schema template and core domain types"
```

### Task 3: Implement auth primitives, repositories, and services

**Files:**
- Create: `internal/auth/password.go`
- Create: `internal/auth/password_test.go`
- Create: `internal/auth/token.go`
- Create: `internal/auth/token_test.go`
- Create: `internal/auth/repository.go`
- Create: `internal/auth/service.go`
- Create: `internal/auth/service_test.go`
- Create: `internal/postgres/auth_store.go`
- Create: `internal/postgres/auth_store_test.go`

**Step 1: Write the failing auth service tests**

```go
func TestAuthenticateCreatesSessionForValidCredentials(t *testing.T) {
	repo := &fakeRepo{user: User{ID: mustUUID(t), Username: "alice", PasswordHash: mustHash(t, "secret-pass")}}
	svc := NewService(repo, 24*time.Hour)

	session, err := svc.AuthenticatePassword(context.Background(), "alice", "secret-pass")
	if err != nil {
		t.Fatalf("AuthenticatePassword() error = %v", err)
	}
	if session.UserID != repo.user.ID {
		t.Fatalf("UserID = %s, want %s", session.UserID, repo.user.ID)
	}
}
```

```go
func TestRequireMCPTokenUserRejectsRevokedToken(t *testing.T) {
	repo := &fakeRepo{tokenErr: ErrTokenRevoked}
	svc := NewService(repo, 24*time.Hour)

	_, err := svc.RequireMCPTokenUser(context.Background(), "revoked-token")
	if !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("error = %v, want ErrTokenRevoked", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/auth ./internal/postgres -run 'TestAuthenticateCreatesSessionForValidCredentials|TestRequireMCPTokenUserRejectsRevokedToken|TestHashTokenDeterministic' -v`
Expected: FAIL because the auth service and storage do not exist.

**Step 3: Write minimal implementation**

- Add password hashing/verification helpers.
- Add token hashing helper for sessions and MCP tokens.
- Add repository methods for finding users, creating/deleting sessions, and looking up token-mapped users.
- Add `Service.AuthenticatePassword`, `Service.RequireSession`, and `Service.RequireMCPTokenUser`.
- Use explicit sentinel errors for invalid credentials, invalid sessions, revoked tokens, and disabled users.
- Reject disabled users both at login time and when resolving existing sessions.
- Add best-effort/throttled session and MCP-token activity updates so `last_seen_at` and `last_used_at` are maintained without a write on every request.

```go
type Repository interface {
	FindUserByUsername(ctx context.Context, username string) (User, error)
	CreateSession(ctx context.Context, params CreateSessionParams) (Session, error)
	DeleteSession(ctx context.Context, sessionID uuid.UUID) error
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error)
	FindUserByMCPTokenHash(ctx context.Context, tokenHash string) (User, MCPToken, error)
	TouchSessionActivity(ctx context.Context, sessionID uuid.UUID, seenAt time.Time) error
	TouchMCPTokenActivity(ctx context.Context, tokenID uuid.UUID, usedAt time.Time) error
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth ./internal/postgres -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/auth/password.go internal/auth/password_test.go internal/auth/token.go internal/auth/token_test.go internal/auth/repository.go internal/auth/service.go internal/auth/service_test.go internal/postgres/auth_store.go internal/postgres/auth_store_test.go
git commit -m "openbrainfun: add auth primitives and storage"
```

### Task 4: Wire login/logout handlers, middleware, and session API

**Files:**
- Create: `internal/web/auth_handlers.go`
- Create: `internal/web/auth_handlers_test.go`
- Create: `internal/api/session_handlers.go`
- Create: `internal/api/session_handlers_test.go`
- Create: `internal/security/csrf.go`
- Create: `internal/security/csrf_test.go`
- Create: `internal/server/web_router.go`
- Create: `internal/server/web_router_test.go`
- Create: `internal/web/templates/login.tmpl`

**Step 1: Write the failing handler tests**

```go
func TestLoginPageShowsForm(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rr := httptest.NewRecorder()

	NewAuthHandlers(fakeAuthService{}).LoginPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "name=\"username\"") || !strings.Contains(body, ">Log in<") {
		t.Fatalf("body missing login form: %s", body)
	}
}
```

```go
func TestCookieAuthenticatedWriteRejectsMissingCSRF(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/thoughts", strings.NewReader(`{"content":"hello"}`))
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, auth.User{ID: mustUUID(t)}))
	rr := httptest.NewRecorder()

	NewCSRFMiddleware("test-csrf-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/web ./internal/api ./internal/security ./internal/server -run 'TestLoginPageShowsForm|TestPostSessionSetsCookie|TestProtectedRouteRedirectsToLogin|TestCookieAuthenticatedWriteRejectsMissingCSRF' -v`
Expected: FAIL because the handlers, template, and middleware do not exist.

**Step 3: Write minimal implementation**

- Add `GET /login`, `POST /login`, and `POST /logout` for browser flows.
- Add `POST /api/session` and `DELETE /api/session` for curl/test flows.
- Set session cookies with the configured `Secure` mode, `HttpOnly`, and `SameSite=Lax`, and redirect unauthenticated browser traffic to `/login`.
- Return JSON auth errors from `/api/session` instead of redirects.
- Add a small auth middleware that resolves the current user once and stores it in request context.
- Return a CSRF token from `/api/session`, render CSRF hidden inputs in HTML forms, and require a CSRF header/token on cookie-authenticated writes.
- Treat disabled or expired sessions as unauthenticated in both browser and JSON API paths.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/web ./internal/api ./internal/security ./internal/server -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/web/auth_handlers.go internal/web/auth_handlers_test.go internal/api/session_handlers.go internal/api/session_handlers_test.go internal/security/csrf.go internal/security/csrf_test.go internal/server/web_router.go internal/server/web_router_test.go internal/web/templates/login.tmpl
git commit -m "openbrainfun: add login and session handling"
```

### Task 5: Implement thought repository and ownership-scoped CRUD, retry, and query services

**Files:**
- Create: `internal/thoughts/repository.go`
- Create: `internal/thoughts/service.go`
- Create: `internal/thoughts/service_test.go`
- Create: `internal/postgres/thought_store.go`
- Create: `internal/postgres/thought_store_test.go`

**Step 1: Write the failing thought service tests**

```go
func TestDeleteThoughtRejectsWrongOwner(t *testing.T) {
	repo := &fakeRepo{thought: Thought{ID: mustUUID(t), UserID: mustUUID(t), Content: "secret"}}
	svc := NewService(repo)

	err := svc.DeleteThought(context.Background(), mustUUID(t), repo.thought.ID)
	if !errors.Is(err, ErrThoughtNotFound) {
		t.Fatalf("error = %v, want ErrThoughtNotFound", err)
	}
}
```

```go
func TestRetryThoughtResetsFailedIngest(t *testing.T) {
	repo := &fakeRepo{thought: Thought{
		ID: mustUUID(t), UserID: mustUUID(t), Content: "secret", IngestStatus: IngestFailed, IngestError: "ollama down",
	}}
	svc := NewService(repo)

	got, err := svc.RetryThought(context.Background(), repo.thought.UserID, repo.thought.ID)
	if err != nil {
		t.Fatalf("RetryThought() error = %v", err)
	}
	if got.IngestStatus != IngestPending || got.IngestError != "" {
		t.Fatalf("got %+v, want pending with cleared error", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/thoughts ./internal/postgres -run 'TestDeleteThoughtRejectsWrongOwner|TestCreateThoughtStoresPendingRecord|TestRetryThoughtResetsFailedIngest' -v`
Expected: FAIL because the repository and service are incomplete.

**Step 3: Write minimal implementation**

- Add repository methods for create/get/update/delete/list/search by `user_id`.
- Add service methods for create, update, delete, get, and list.
- Add retry-ingest service/repository behavior that resets failed thoughts back to `pending`.
- Make delete a hard delete.
- Reset `ingest_status` to `pending` when searchable fields change.
- Return not-found for wrong-owner access so ownership does not leak.
- Add explicit query parameter structs for `q`, `search_mode`, `exposure_scope`, `ingest_status`, `tag`, `page`, and `page_size`.
- Keep non-search browse order deterministic as `updated_at desc, id desc`.
- Reserve a `SearchSemantic` repository/query path so the API, UI, and MCP handlers can share one search contract once the embedder is wired in.

```go
type Repository interface {
	CreateThought(ctx context.Context, params CreateThoughtParams) (Thought, error)
	GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error)
	UpdateThought(ctx context.Context, params UpdateThoughtParams) (Thought, error)
	DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error
	ListThoughts(ctx context.Context, params ListThoughtsParams) ([]Thought, error)
	SearchKeyword(ctx context.Context, params SearchKeywordParams) ([]Thought, error)
	SearchSemantic(ctx context.Context, params SearchSemanticParams) ([]Thought, error)
	RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error)
	ClaimPending(ctx context.Context, limit int) ([]Thought, error)
	MarkReady(ctx context.Context, params MarkReadyParams) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/thoughts ./internal/postgres -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/thoughts/repository.go internal/thoughts/service.go internal/thoughts/service_test.go internal/postgres/thought_store.go internal/postgres/thought_store_test.go
git commit -m "openbrainfun: add ownership-scoped thought CRUD"
```

### Task 6: Add the embedder interface, fake backends, and normalized metadata extractor types

**Files:**
- Create: `internal/embed/embedder.go`
- Create: `internal/embed/fake.go`
- Create: `internal/embed/fake_test.go`
- Create: `internal/embed/contract_test.go`
- Create: `internal/embed/testsuite.go`
- Create: `internal/metadata/extractor.go`
- Create: `internal/metadata/extractor_test.go`

**Step 1: Write the failing embedder contract test**

```go
func TestFakeEmbedderSatisfiesContract(t *testing.T) {
	RunContractSuite(t, func(t *testing.T) Embedder {
		return NewFake(map[string][]float32{
			"mcp auth note":   []float32{1, 0, 0},
			"totally unrelated": []float32{0, 1, 0},
		})
	})
}
```

```go
func TestFakeExtractorNormalizesOutput(t *testing.T) {
	extractor := metadata.NewFake(map[string]metadata.Metadata{
		"remember mcp auth": {Summary: "remember mcp auth", Topics: []string{"mcp", "auth"}},
	})
	got, err := extractor.Extract(context.Background(), "remember mcp auth")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(got.Topics) != 2 {
		t.Fatalf("Topics = %v, want normalized topics", got.Topics)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/embed ./internal/metadata -run 'TestFakeEmbedderSatisfiesContract|TestFakeExtractorNormalizesOutput' -v`
Expected: FAIL because the interfaces, fakes, and shared contract/normalization code do not exist.

**Step 3: Write minimal implementation**

- Define a small `Embedder` interface with batch embedding.
- Add a deterministic fake embedder for unit/integration tests.
- Add a small `metadata.Extractor` interface and deterministic fake extractor for tests.
- Add a shared contract suite that checks non-empty vectors, stable dimensions, finite values, and simple similarity expectations.
- Keep the contract assertions backend-agnostic enough to run against both fake and real providers.
- Keep metadata normalization centralized so fake and real extractors both persist the same shape.

```go
type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
	Dimensions() int
	Model() string
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed ./internal/metadata -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/embed/embedder.go internal/embed/fake.go internal/embed/fake_test.go internal/embed/contract_test.go internal/embed/testsuite.go internal/metadata/extractor.go internal/metadata/extractor_test.go
git commit -m "openbrainfun: add fake embedding and metadata backends"
```

### Task 7: Add the Ollama embedder, metadata extractor, model probing, and background worker

**Files:**
- Create: `internal/ollama/client.go`
- Create: `internal/ollama/client_test.go`
- Create: `internal/ollama/provider.go`
- Create: `internal/ollama/provider_test.go`
- Create: `internal/ollama/metadata_provider.go`
- Create: `internal/ollama/metadata_provider_test.go`
- Create: `internal/worker/processor.go`
- Create: `internal/worker/processor_test.go`
- Modify: `cmd/openbrain/main.go`
- Modify: `internal/embed/contract_test.go`

**Step 1: Write the failing worker test**

```go
func TestProcessorMarksThoughtReadyAfterEmbeddingAndMetadataExtraction(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: mustUUID(t), UserID: mustUUID(t), Content: "remember mcp auth", IngestStatus: thoughts.IngestPending}}}
	processor := NewProcessor(
		repo,
		fakeEmbedder{vectors: [][]float32{{0.1, 0.2, 0.3}}},
		fakeMetadataExtractor{metadata: metadata.Metadata{Summary: "remember mcp auth", Topics: []string{"mcp", "auth"}}},
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ollama ./internal/worker -run 'TestProcessorMarksThoughtReadyAfterEmbeddingAndMetadataExtraction|TestOllamaProviderSatisfiesContract|TestMetadataProviderNormalizesJSON' -v`
Expected: FAIL because the Ollama providers, probe logic, metadata extraction, and worker do not exist.

**Step 3: Write minimal implementation**

- Add an Ollama client that calls `/api/embed` and can probe the configured model for dimensions.
- Add an `ollama.Provider` that satisfies `embed.Embedder`.
- Add an Ollama metadata provider that requests structured JSON and passes it through `metadata.Normalize`.
- Extend the shared embedder contract so it can optionally run against the real provider when `OPENBRAIN_EMBED_BACKEND=ollama` is set.
- Add `worker.Processor.RunOnce` and a small polling loop from `main.go`.
- Persist normalized metadata when extraction succeeds and safe defaults when extraction fails after embeddings succeed.
- Mark thoughts `failed` on terminal embed errors and `ready` when embeddings succeed.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/ollama ./internal/worker -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ollama/client.go internal/ollama/client_test.go internal/ollama/provider.go internal/ollama/provider_test.go internal/ollama/metadata_provider.go internal/ollama/metadata_provider_test.go internal/worker/processor.go internal/worker/processor_test.go cmd/openbrain/main.go internal/embed/contract_test.go
git commit -m "openbrainfun: add ollama embedding and metadata worker"
```

### Task 8: Build the server-rendered responsive UI

**Files:**
- Create: `internal/web/handlers.go`
- Create: `internal/web/handlers_test.go`
- Create: `internal/web/templates/layout.tmpl`
- Create: `internal/web/templates/index.tmpl`
- Create: `internal/web/templates/thoughts.tmpl`
- Create: `internal/web/templates/thought.tmpl`
- Create: `internal/web/templates/delete_confirm.tmpl`
- Create: `static/app.css`
- Modify: `internal/server/web_router.go`

**Step 1: Write the failing UI tests**

```go
func TestIndexPageShowsCaptureFormAndSubmitButton(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	NewHandlers(fakeThoughtService{}).Index(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "name=\"content\"") || !strings.Contains(body, ">Save thought<") {
		t.Fatalf("body missing capture controls: %s", body)
	}
}
```

```go
func TestThoughtPageShowsExtractedMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/thoughts/123", nil)
	rr := httptest.NewRecorder()

	NewHandlers(fakeThoughtService{thought: thoughts.Thought{
		ID: mustUUID(t),
		Metadata: metadata.Metadata{Summary: "remember mcp auth", Topics: []string{"mcp", "auth"}},
	}}).ThoughtDetail(rr, req)

	if !strings.Contains(rr.Body.String(), "remember mcp auth") {
		t.Fatalf("body missing extracted metadata: %s", rr.Body.String())
	}
}
```

```go
func TestThoughtsPageShowsRetryActionForFailedThought(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/thoughts", nil)
	rr := httptest.NewRecorder()

	NewHandlers(fakeThoughtService{list: []thoughts.Thought{{
		ID: mustUUID(t),
		IngestStatus: thoughts.IngestFailed,
		IngestError: "ollama down",
	}}}).Thoughts(rr, req)

	if !strings.Contains(rr.Body.String(), "Retry ingestion") {
		t.Fatalf("body missing retry action: %s", rr.Body.String())
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/web -run 'TestIndexPageShowsCaptureFormAndSubmitButton|TestThoughtPageShowsDeleteButton|TestThoughtPageShowsExtractedMetadata|TestThoughtsPageShowsRetryActionForFailedThought' -v`
Expected: FAIL because the handlers, templates, and styles do not exist.

**Step 3: Write minimal implementation**

- Add server-rendered pages for `/`, `/thoughts`, `/thoughts/{id}`, and `/thoughts/{id}/delete`.
- Add a retry action on `/thoughts` and `/thoughts/{id}` for failed ingest.
- Add visible `Save thought`, `Save changes`, and `Delete thought` buttons.
- Add responsive CSS that keeps forms readable on mobile and desktop.
- Keep the pages functional without JavaScript.
- Surface flash messages, ingest status, and extracted metadata visibly.
- Add explicit empty states and preserve search/filter/pagination state in the URL.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/web -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/web/handlers.go internal/web/handlers_test.go internal/web/templates/layout.tmpl internal/web/templates/index.tmpl internal/web/templates/thoughts.tmpl internal/web/templates/thought.tmpl internal/web/templates/delete_confirm.tmpl static/app.css internal/server/web_router.go
git commit -m "openbrainfun: add responsive authenticated web ui"
```

### Task 9: Add the authenticated JSON thought API

**Files:**
- Create: `internal/api/thought_handlers.go`
- Create: `internal/api/thought_handlers_test.go`
- Modify: `internal/server/web_router.go`

**Step 1: Write the failing API test**

```go
func TestPostThoughtReturnsCreatedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/thoughts", strings.NewReader(`{"content":"Remember MCP auth","exposure_scope":"remote_ok"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, auth.User{ID: mustUUID(t)}))
	rr := httptest.NewRecorder()

	NewThoughtHandlers(fakeThoughtService{}).Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
}
```

```go
func TestListThoughtsSupportsSearchParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/thoughts?q=mcp&search_mode=semantic&page=2&page_size=10", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, auth.User{ID: mustUUID(t)}))
	rr := httptest.NewRecorder()

	NewThoughtHandlers(fakeThoughtService{}).List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}
```

```go
func TestRetryThoughtRequiresCSRFToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/thoughts/123/retry", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey{}, auth.User{ID: mustUUID(t)}))
	rr := httptest.NewRecorder()

	NewThoughtHandlers(fakeThoughtService{}).Retry(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run 'TestPostThoughtReturnsCreatedJSON|TestDeleteThoughtReturnsNoContent|TestRetryThoughtRequiresCSRFToken|TestListThoughtsSupportsSearchParams' -v`
Expected: FAIL because the JSON CRUD handlers do not exist.

**Step 3: Write minimal implementation**

- Add `POST /api/thoughts`, `GET /api/thoughts/{id}`, `PATCH /api/thoughts/{id}`, `DELETE /api/thoughts/{id}`, `POST /api/thoughts/{id}/retry`, and `GET /api/thoughts`.
- Reuse the same thought service and ownership checks as the HTML handlers.
- Return structured JSON for errors and resources.
- Preserve cookie-based auth for curl via `/api/session`.
- Require CSRF headers on cookie-authenticated write endpoints.
- Parse `q`, `search_mode`, `exposure_scope`, `ingest_status`, `tag`, `page`, and `page_size` on `GET /api/thoughts`.
- Route keyword and semantic search through one shared query contract so JSON API and MCP behave consistently.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/api -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/api/thought_handlers.go internal/api/thought_handlers_test.go internal/server/web_router.go
git commit -m "openbrainfun: add authenticated thought json api"
```

### Task 10: Add the separate-port MCP server with per-user token isolation

**Files:**
- Create: `internal/mcpserver/server.go`
- Create: `internal/mcpserver/server_test.go`
- Create: `internal/mcpserver/tools.go`
- Create: `internal/mcpserver/tools_test.go`
- Create: `internal/server/mcp_router.go`
- Modify: `internal/server/server.go`

**Step 1: Write the failing MCP isolation test**

```go
func TestSearchThoughtsReturnsOnlyMappedUsersRemoteOKThoughts(t *testing.T) {
	repo := &fakeRepo{results: []thoughts.Thought{
		{ID: mustUUID(t), UserID: mustUUID(t), Content: "remote visible", ExposureScope: thoughts.ExposureRemoteOK},
		{ID: mustUUID(t), UserID: mustUUID(t), Content: "local secret", ExposureScope: thoughts.ExposureLocalOnly},
	}}
	srv := New(fakeAuthService{}, repo)

	got, err := srv.SearchThoughts(context.Background(), auth.User{ID: repo.results[0].UserID}, SearchThoughtsInput{Query: "remote"})
	if err != nil {
		t.Fatalf("SearchThoughts() error = %v", err)
	}
	if len(got) != 1 || got[0].ExposureScope != thoughts.ExposureRemoteOK {
		t.Fatalf("got %+v, want one remote_ok result", got)
	}
}
```

```go
func TestMCPMiddlewareRejectsRevokedToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer revoked-token")
	rr := httptest.NewRecorder()

	NewAuthMiddleware(fakeAuthService{tokenErr: auth.ErrTokenRevoked})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/mcpserver ./internal/server -run 'TestSearchThoughtsReturnsOnlyMappedUsersRemoteOKThoughts|TestMCPListenerUsesSeparateAddr|TestMCPMiddlewareRejectsRevokedToken' -v`
Expected: FAIL because the MCP server, tools, and listener wiring do not exist.

**Step 3: Write minimal implementation**

- Add token-auth middleware for the MCP listener.
- Use the official MCP Go SDK.
- Expose `search_thoughts`, `recent_thoughts`, `get_thought`, and `stats`.
- Filter by `user_id` first and `remote_ok` second on every query.
- Reuse the same query service/search contract as the JSON API so keyword vs semantic behavior stays aligned.
- Reject revoked tokens on every request and update `last_used_at` best-effort.
- Configure the server on the dedicated MCP address.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/mcpserver ./internal/server -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/mcpserver/server.go internal/mcpserver/server_test.go internal/mcpserver/tools.go internal/mcpserver/tools_test.go internal/server/mcp_router.go internal/server/server.go
git commit -m "openbrainfun: add isolated mcp server"
```

### Task 11: Add local operations, persistent bind mounts, and README walkthrough generation

**Files:**
- Create: `compose.yaml`
- Create: `scripts/wait-for-stack.sh`
- Create: `scripts/provision-demo-data.sh`
- Create: `scripts/walkthrough.sh`
- Create: `internal/readmegen/render.go`
- Create: `internal/readmegen/render_test.go`
- Create: `docs/operations.md`
- Create: `docs/walkthrough.demo.md`
- Create: `README.md`

**Step 1: Write the failing README render test**

```go
func TestRenderWalkthroughIncludesCurlAndMCPSections(t *testing.T) {
	md, err := RenderWalkthrough(Transcript{
		StoragePaths: []string{"./var/postgres", "./var/ollama"},
		Steps: []Step{{Title: "Create a thought", Command: "curl ... /api/thoughts", Response: "201 Created"}, {Title: "Query MCP", Command: "curl ... :8081/mcp", Response: "200 OK"}},
	})
	if err != nil {
		t.Fatalf("RenderWalkthrough() error = %v", err)
	}
	if !strings.Contains(md, "./var/postgres") || !strings.Contains(md, "Query MCP") {
		t.Fatalf("walkthrough missing required sections: %s", md)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/readmegen -run TestRenderWalkthroughIncludesCurlAndMCPSections -v`
Expected: FAIL because the README renderer and walkthrough assets do not exist.

**Step 3: Write minimal implementation**

- Add `compose.yaml` with explicit bind mounts `./var/postgres` and `./var/ollama`.
- Add a provisioning script that inserts a demo user and a demo MCP token into Postgres.
- Add a walkthrough script that:
  - starts the stack
  - waits for readiness
  - provisions the demo user/token
  - logs in with curl using `/api/session`
  - stores the returned CSRF token alongside the cookie jar
  - creates a thought with curl using both the cookie jar and CSRF header
  - waits for background processing to finish
  - retrieves it with curl
  - calls the MCP endpoint with curl
  - captures requests and responses into `docs/walkthrough.demo.md`
- Make the retrieved thought example show extracted metadata in at least one JSON response.
- Add a README renderer/updater that writes the walkthrough section between markers in `README.md`.
- Keep the README explicit about persistence paths and the exact Ollama model used.
- Add `docs/operations.md` covering persistence paths, local backup/restore expectations, and the operator steps for changing embedding models and re-embedding/reindexing stored thoughts.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/readmegen -v`
Expected: PASS

**Step 5: Run the walkthrough locally**

Run: `./scripts/walkthrough.sh`
Expected: `docs/walkthrough.demo.md` and the README walkthrough section update from real command output.

**Step 6: Commit**

```bash
git add compose.yaml scripts/wait-for-stack.sh scripts/provision-demo-data.sh scripts/walkthrough.sh internal/readmegen/render.go internal/readmegen/render_test.go docs/operations.md docs/walkthrough.demo.md README.md
git commit -m "openbrainfun: add real walkthrough and local ops docs"
```

### Task 12: Add verification scripts, acceptance checklist, backend-agnostic end-to-end tests, and GitHub Actions verification

**Files:**
- Create: `internal/e2e/app_test.go`
- Create: `internal/e2e/testenv.go`
- Create: `scripts/verify.sh`
- Create: `scripts/check-coverage.sh`
- Create: `docs/acceptance-checklist.md`
- Create: `.github/workflows/ci.yml`
- Create: `.github/scripts/run-real-embed-tests.sh`
- Modify: `README.md`

**Step 1: Write the failing backend-agnostic E2E test**

```go
func TestEndToEndCRUDAndMCPIsolation(t *testing.T) {
	env := NewTestEnv(t)
	client := env.LoginAsDemoUser(t)
	thought := env.CreateThought(t, client, CreateThoughtRequest{Content: "Remember MCP auth", ExposureScope: "remote_ok"})
	got := env.GetThought(t, client, thought.ID)
	if len(got.Metadata.Topics) == 0 {
		t.Fatal("expected extracted metadata")
	}
	env.AssertMCPFindsThought(t, env.DemoToken(), "MCP auth")
	env.AssertOtherUsersMCPTokenCannotSeeThought(t, thought.ID)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/e2e -run TestEndToEndCRUDAndMCPIsolation -v`
Expected: FAIL because the reusable test environment and app harness do not exist.

**Step 3: Write minimal implementation**

- Add an E2E harness that can boot the app against Postgres and either the fake or real embedder.
- Add fake and real metadata-extraction wiring to the same harness.
- Make the harness choose backend via `OPENBRAIN_EMBED_BACKEND`.
- Add `scripts/check-coverage.sh` to enforce repo-wide and core-package coverage floors.
- Add `scripts/verify.sh` to run `gofmt`, `go vet`, tests, and coverage checks locally in the same way CI does.
- Add `docs/acceptance-checklist.md` mapping requirements to automated tests, walkthrough output, and any remaining manual checks.
- Add `.github/workflows/ci.yml` with two required jobs:
  - fake backend job: `./scripts/verify.sh`
  - real backend job: start Postgres, install/start Ollama, pull `all-minilm:22m`, run the shared contract/E2E suites, run a metadata-extraction smoke test, run `./scripts/walkthrough.sh`, and verify the README walkthrough stays in sync
- Fail CI if coverage drops below the agreed floor.

**Step 4: Run local verification commands**

Run: `./scripts/verify.sh`
Expected: PASS, including formatting, `go vet`, tests, and coverage checks

Run: `./scripts/check-coverage.sh /tmp/cover.out`
Expected: PASS, with repo-wide and core-package floors enforced consistently with CI

**Step 5: Commit**

```bash
git add internal/e2e/app_test.go internal/e2e/testenv.go scripts/verify.sh scripts/check-coverage.sh docs/acceptance-checklist.md .github/workflows/ci.yml .github/scripts/run-real-embed-tests.sh README.md
git commit -m "openbrainfun: add ci verification for fake and real embed backends"
```
