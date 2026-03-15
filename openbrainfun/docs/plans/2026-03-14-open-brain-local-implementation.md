# Local Open Brain Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a multi-user local-first open-brain service with DB-backed login sessions, per-user thought ownership, per-user MCP token access on a separate port, a generated README walkthrough, and CI that verifies both fake and real Ollama embedding backends.

**Architecture:** One Go binary serves the browser UI and JSON API on the web listener and the read-only MCP server on a second listener. Postgres stores users, sessions, MCP tokens, thoughts, and ingest state. Ollama supplies embeddings behind a small interface so fake and real providers can run through the same contract/integration suites.

**Tech Stack:** Go, `net/http`, `html/template`, `github.com/jackc/pgx/v5`, `github.com/modelcontextprotocol/go-sdk`, Postgres, pgvector, pgcrypto, Ollama, Docker Compose, GitHub Actions.

---

## Implementation notes before starting

- Keep application code at the repository root (`cmd/`, `internal/`, `migrations/`, `scripts/`, `README.md`).
- The README walkthrough is a deliverable, not a follow-up. Design the code so the walkthrough can be generated from real interactions.
- Use `all-minilm:22m` for the README quickstart and the real-provider CI job. Allow `OPENBRAIN_EMBED_MODEL` to override it.
- Do not hardcode embedding dimension blindly. Probe the configured Ollama model and render the migration with the probed dimension before applying it.
- The web listener and MCP listener must be separate addresses from day one.
- The same contract/integration suites must be runnable with `OPENBRAIN_EMBED_BACKEND=fake` and `OPENBRAIN_EMBED_BACKEND=ollama`.

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
	t.Setenv("OPENBRAIN_SESSION_TTL", "24h")

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
- Add `Config` with web/MCP addresses, database URL, Ollama URL, embed model, session TTL, and log level.
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
	SessionTTL   time.Duration
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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/migrations ./internal/auth ./internal/thoughts -run 'TestRenderInitialSchemaReplacesEmbedDimensions|TestNewThoughtRejectsBlankContent' -v`
Expected: FAIL because the render function and domain types do not exist.

**Step 3: Write minimal implementation**

- Add the SQL template containing `users`, `web_sessions`, `mcp_tokens`, and `thoughts`.
- Add `RenderSchema(template string, embedDimensions int)`.
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

Run: `go test ./internal/migrations ./internal/auth ./internal/thoughts -v`
Expected: PASS

**Step 5: Commit**

```bash
git add migrations/0001_initial.sql.tmpl internal/migrations/render.go internal/migrations/render_test.go internal/auth/types.go internal/auth/types_test.go internal/thoughts/types.go internal/thoughts/types_test.go scripts/render-migration.sh
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

**Step 1: Write the failing auth service test**

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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/auth ./internal/postgres -run 'TestAuthenticateCreatesSessionForValidCredentials|TestHashTokenDeterministic' -v`
Expected: FAIL because the auth service and storage do not exist.

**Step 3: Write minimal implementation**

- Add password hashing/verification helpers.
- Add token hashing helper for sessions and MCP tokens.
- Add repository methods for finding users, creating/deleting sessions, and looking up token-mapped users.
- Add `Service.AuthenticatePassword`, `Service.RequireSession`, and `Service.RequireMCPTokenUser`.
- Use explicit sentinel errors for invalid credentials, invalid sessions, revoked tokens, and disabled users.

```go
type Repository interface {
	FindUserByUsername(ctx context.Context, username string) (User, error)
	CreateSession(ctx context.Context, params CreateSessionParams) (Session, error)
	DeleteSession(ctx context.Context, sessionID uuid.UUID) error
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error)
	FindUserByMCPTokenHash(ctx context.Context, tokenHash string) (User, MCPToken, error)
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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/web ./internal/api ./internal/server -run 'TestLoginPageShowsForm|TestPostSessionSetsCookie|TestProtectedRouteRedirectsToLogin' -v`
Expected: FAIL because the handlers, template, and middleware do not exist.

**Step 3: Write minimal implementation**

- Add `GET /login`, `POST /login`, and `POST /logout` for browser flows.
- Add `POST /api/session` and `DELETE /api/session` for curl/test flows.
- Set secure session cookies and redirect unauthenticated browser traffic to `/login`.
- Return JSON auth errors from `/api/session` instead of redirects.
- Add a small auth middleware that resolves the current user once and stores it in request context.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/web ./internal/api ./internal/server -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/web/auth_handlers.go internal/web/auth_handlers_test.go internal/api/session_handlers.go internal/api/session_handlers_test.go internal/server/web_router.go internal/server/web_router_test.go internal/web/templates/login.tmpl
git commit -m "openbrainfun: add login and session handling"
```

### Task 5: Implement thought repository and ownership-scoped CRUD services

**Files:**
- Create: `internal/thoughts/repository.go`
- Create: `internal/thoughts/service.go`
- Create: `internal/thoughts/service_test.go`
- Create: `internal/postgres/thought_store.go`
- Create: `internal/postgres/thought_store_test.go`

**Step 1: Write the failing thought service test**

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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/thoughts ./internal/postgres -run 'TestDeleteThoughtRejectsWrongOwner|TestCreateThoughtStoresPendingRecord' -v`
Expected: FAIL because the repository and service are incomplete.

**Step 3: Write minimal implementation**

- Add repository methods for create/get/update/delete/list/search by `user_id`.
- Add service methods for create, update, delete, get, and list.
- Make delete a hard delete.
- Reset `ingest_status` to `pending` when searchable fields change.
- Return not-found for wrong-owner access so ownership does not leak.

```go
type Repository interface {
	CreateThought(ctx context.Context, params CreateThoughtParams) (Thought, error)
	GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (Thought, error)
	UpdateThought(ctx context.Context, params UpdateThoughtParams) (Thought, error)
	DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error
	ListThoughts(ctx context.Context, params ListThoughtsParams) ([]Thought, error)
	SearchKeyword(ctx context.Context, params SearchKeywordParams) ([]Thought, error)
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

### Task 6: Add the embedder interface, fake embedder, and shared contract suite

**Files:**
- Create: `internal/embed/embedder.go`
- Create: `internal/embed/fake.go`
- Create: `internal/embed/fake_test.go`
- Create: `internal/embed/contract_test.go`
- Create: `internal/embed/testsuite.go`

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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/embed -run TestFakeEmbedderSatisfiesContract -v`
Expected: FAIL because the interface, fake, and shared contract suite do not exist.

**Step 3: Write minimal implementation**

- Define a small `Embedder` interface with batch embedding.
- Add a deterministic fake embedder for unit/integration tests.
- Add a shared contract suite that checks non-empty vectors, stable dimensions, finite values, and simple similarity expectations.
- Keep the contract assertions backend-agnostic enough to run against both fake and real providers.

```go
type Embedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
	Dimensions() int
	Model() string
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/embed/embedder.go internal/embed/fake.go internal/embed/fake_test.go internal/embed/contract_test.go internal/embed/testsuite.go
git commit -m "openbrainfun: add embedder interface and fake backend"
```

### Task 7: Add the Ollama embedder, model probing, and background worker

**Files:**
- Create: `internal/ollama/client.go`
- Create: `internal/ollama/client_test.go`
- Create: `internal/ollama/provider.go`
- Create: `internal/ollama/provider_test.go`
- Create: `internal/worker/processor.go`
- Create: `internal/worker/processor_test.go`
- Modify: `cmd/openbrain/main.go`
- Modify: `internal/embed/contract_test.go`

**Step 1: Write the failing worker test**

```go
func TestProcessorMarksThoughtReadyAfterEmbedding(t *testing.T) {
	repo := &fakeRepo{pending: []thoughts.Thought{{ID: mustUUID(t), UserID: mustUUID(t), Content: "remember mcp auth", IngestStatus: thoughts.IngestPending}}}
	processor := NewProcessor(repo, fakeEmbedder{vectors: [][]float32{{0.1, 0.2, 0.3}}})

	if err := processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repo.readyCalls) != 1 {
		t.Fatalf("readyCalls = %d, want 1", len(repo.readyCalls))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/ollama ./internal/worker -run 'TestProcessorMarksThoughtReadyAfterEmbedding|TestOllamaProviderSatisfiesContract' -v`
Expected: FAIL because the Ollama provider, probe logic, and worker do not exist.

**Step 3: Write minimal implementation**

- Add an Ollama client that calls `/api/embed` and can probe the configured model for dimensions.
- Add an `ollama.Provider` that satisfies `embed.Embedder`.
- Extend the shared embedder contract so it can optionally run against the real provider when `OPENBRAIN_EMBED_BACKEND=ollama` is set.
- Add `worker.Processor.RunOnce` and a small polling loop from `main.go`.
- Mark thoughts `failed` on terminal embed errors and `ready` on success.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/ollama ./internal/worker -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ollama/client.go internal/ollama/client_test.go internal/ollama/provider.go internal/ollama/provider_test.go internal/worker/processor.go internal/worker/processor_test.go cmd/openbrain/main.go internal/embed/contract_test.go
git commit -m "openbrainfun: add ollama embedder and worker"
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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/web -run 'TestIndexPageShowsCaptureFormAndSubmitButton|TestThoughtPageShowsDeleteButton' -v`
Expected: FAIL because the handlers, templates, and styles do not exist.

**Step 3: Write minimal implementation**

- Add server-rendered pages for `/`, `/thoughts`, `/thoughts/{id}`, and `/thoughts/{id}/delete`.
- Add visible `Save thought`, `Save changes`, and `Delete thought` buttons.
- Add responsive CSS that keeps forms readable on mobile and desktop.
- Keep the pages functional without JavaScript.
- Surface flash messages and ingest status visibly.

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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/api -run 'TestPostThoughtReturnsCreatedJSON|TestDeleteThoughtReturnsNoContent' -v`
Expected: FAIL because the JSON CRUD handlers do not exist.

**Step 3: Write minimal implementation**

- Add `POST /api/thoughts`, `GET /api/thoughts/{id}`, `PATCH /api/thoughts/{id}`, `DELETE /api/thoughts/{id}`, and `GET /api/thoughts`.
- Reuse the same thought service and ownership checks as the HTML handlers.
- Return structured JSON for errors and resources.
- Preserve cookie-based auth for curl via `/api/session`.

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

**Step 2: Run test to verify it fails**

Run: `go test ./internal/mcpserver ./internal/server -run 'TestSearchThoughtsReturnsOnlyMappedUsersRemoteOKThoughts|TestMCPListenerUsesSeparateAddr' -v`
Expected: FAIL because the MCP server, tools, and listener wiring do not exist.

**Step 3: Write minimal implementation**

- Add token-auth middleware for the MCP listener.
- Use the official MCP Go SDK.
- Expose `search_thoughts`, `recent_thoughts`, `get_thought`, and `stats`.
- Filter by `user_id` first and `remote_ok` second on every query.
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
  - creates a thought with curl
  - retrieves it with curl
  - calls the MCP endpoint with curl
  - captures requests and responses into `docs/walkthrough.demo.md`
- Add a README renderer/updater that writes the walkthrough section between markers in `README.md`.
- Keep the README explicit about persistence paths and the exact Ollama model used.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/readmegen -v`
Expected: PASS

**Step 5: Run the walkthrough locally**

Run: `./scripts/walkthrough.sh`
Expected: `docs/walkthrough.demo.md` and the README walkthrough section update from real command output.

**Step 6: Commit**

```bash
git add compose.yaml scripts/wait-for-stack.sh scripts/provision-demo-data.sh scripts/walkthrough.sh internal/readmegen/render.go internal/readmegen/render_test.go docs/walkthrough.demo.md README.md
git commit -m "openbrainfun: add real walkthrough and local ops docs"
```

### Task 12: Add backend-agnostic end-to-end tests and GitHub Actions verification

**Files:**
- Create: `internal/e2e/app_test.go`
- Create: `internal/e2e/testenv.go`
- Create: `.github/workflows/ci.yml`
- Create: `.github/scripts/run-real-embed-tests.sh`
- Modify: `README.md`

**Step 1: Write the failing backend-agnostic E2E test**

```go
func TestEndToEndCRUDAndMCPIsolation(t *testing.T) {
	env := NewTestEnv(t)
	client := env.LoginAsDemoUser(t)
	thought := env.CreateThought(t, client, CreateThoughtRequest{Content: "Remember MCP auth", ExposureScope: "remote_ok"})
	env.GetThought(t, client, thought.ID)
	env.AssertMCPFindsThought(t, env.DemoToken(), "MCP auth")
	env.AssertOtherUsersMCPTokenCannotSeeThought(t, thought.ID)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/e2e -run TestEndToEndCRUDAndMCPIsolation -v`
Expected: FAIL because the reusable test environment and app harness do not exist.

**Step 3: Write minimal implementation**

- Add an E2E harness that can boot the app against Postgres and either the fake or real embedder.
- Make the harness choose backend via `OPENBRAIN_EMBED_BACKEND`.
- Add `.github/workflows/ci.yml` with two required jobs:
  - fake backend job: `gofmt`, `go vet`, unit tests, integration tests, coverage gate
  - real backend job: start Postgres, install/start Ollama, pull `all-minilm:22m`, run the shared contract/E2E suites, run `./scripts/walkthrough.sh`, and verify the README walkthrough stays in sync
- Fail CI if coverage drops below the agreed floor.

**Step 4: Run local verification commands**

Run: `gofmt -w $(fd -e go .)`
Expected: files formatted with no diffs left behind after re-running

Run: `go vet ./...`
Expected: PASS

Run: `go test ./... -coverprofile=/tmp/cover.out`
Expected: PASS

Run: `go tool cover -func=/tmp/cover.out`
Expected: repo-wide coverage meets the configured floor and core packages are visibly high.

**Step 5: Commit**

```bash
git add internal/e2e/app_test.go internal/e2e/testenv.go .github/workflows/ci.yml .github/scripts/run-real-embed-tests.sh README.md
git commit -m "openbrainfun: add ci verification for fake and real embed backends"
```
