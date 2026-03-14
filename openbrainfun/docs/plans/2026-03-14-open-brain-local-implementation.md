# Local Open Brain Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a local-first open-brain service with a Go web app, Postgres + pgvector storage, Ollama-based embeddings, a deterministic browse/search UI, and a public read-only MCP endpoint filtered by server-side exposure policy.

**Architecture:** One Go service owns the HTTP UI, JSON API, MCP transport, and a small internal background worker loop. Postgres is the system of record; Ollama supplies embeddings; the reverse proxy terminates TLS and fronts the service publicly while the app enforces bearer auth for MCP and visibility filtering for every remote query.

**Tech Stack:** Go, `net/http`, `html/template`, `github.com/jackc/pgx/v5`, `github.com/modelcontextprotocol/go-sdk`, Postgres, pgvector, Ollama, Docker Compose for local dependencies.

---

## Implementation notes before starting

- Keep the codebase under `openbrainfun/`.
- Prefer one Go module and one binary with an internal worker loop for v1.
- Before writing the migration, probe the chosen Ollama embedding model once and lock the vector dimension in the migration. According to Ollama’s docs, dimensions vary by model, typically 384–1024. Source: <https://docs.ollama.com/capabilities/embeddings>
- Open WebUI’s native MCP support expects **Streamable HTTP**, not stdio or SSE. Source: <https://docs.openwebui.com/features/mcp>
- The official MCP Go SDK should be used unless a verified blocker appears during the initial transport spike. Source: <https://github.com/modelcontextprotocol/go-sdk>

### Task 1: Bootstrap the Go module and configuration skeleton

**Files:**
- Create: `openbrainfun/go.mod`
- Create: `openbrainfun/go.sum`
- Create: `openbrainfun/cmd/openbrain/main.go`
- Create: `openbrainfun/internal/config/config.go`
- Create: `openbrainfun/internal/config/config_test.go`
- Create: `openbrainfun/README.md`

**Step 1: Create the failing config test**

```go
func TestLoadFromEnv(t *testing.T) {
	t.Setenv("OPENBRAIN_HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("OPENBRAIN_DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/openbrain?sslmode=disable")
	t.Setenv("OPENBRAIN_OLLAMA_URL", "http://127.0.0.1:11434")
	t.Setenv("OPENBRAIN_EMBED_MODEL", "embeddinggemma")
	t.Setenv("OPENBRAIN_MCP_BEARER_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.EmbedModel != "embeddinggemma" {
		t.Fatalf("EmbedModel = %q, want embeddinggemma", cfg.EmbedModel)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestLoadFromEnv -v`

Expected: FAIL because the module and `Load` do not exist yet.

**Step 3: Write minimal implementation**

- Initialize the module.
- Add `Config` with `HTTPAddr`, `DatabaseURL`, `OllamaURL`, `EmbedModel`, `EmbedDimensions`, `MCPBearerToken`, and `LogLevel`.
- Add `Load()` that validates required environment variables.
- Add a minimal `main.go` that loads config and starts an empty HTTP server with a `/healthz` route.

```go
type Config struct {
	HTTPAddr       string
	DatabaseURL    string
	OllamaURL      string
	EmbedModel     string
	EmbedDimensions int
	MCPBearerToken string
	LogLevel       string
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run TestLoadFromEnv -v`

Expected: PASS

**Step 5: Commit**

```bash
git add openbrainfun/go.mod openbrainfun/go.sum openbrainfun/cmd/openbrain/main.go openbrainfun/internal/config/config.go openbrainfun/internal/config/config_test.go openbrainfun/README.md
git commit -m "openbrainfun: bootstrap module and config"
```

### Task 2: Add schema, migrations, and repository contracts

**Files:**
- Create: `openbrainfun/migrations/0001_initial.sql`
- Create: `openbrainfun/internal/thoughts/types.go`
- Create: `openbrainfun/internal/thoughts/repository.go`
- Create: `openbrainfun/internal/thoughts/types_test.go`
- Create: `openbrainfun/internal/postgres/store.go`
- Create: `openbrainfun/internal/postgres/store_test.go`

**Step 1: Write the failing domain test**

```go
func TestNewThoughtRejectsBlankContent(t *testing.T) {
	_, err := NewThought(NewThoughtParams{Content: "   "})
	if err == nil {
		t.Fatal("expected error for blank content")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/thoughts -run TestNewThoughtRejectsBlankContent -v`

Expected: FAIL because the domain constructor and types do not exist.

**Step 3: Write minimal implementation**

- Define `Thought`, `ExposureScope`, and `IngestStatus` types.
- Add `NewThought(params)` validation.
- Add repository interfaces for insert, update-ingest-result, list recent, keyword search, and semantic search.
- Write `0001_initial.sql` with the table and indexes from the design doc, leaving a `TODO` comment beside the vector dimension if it has not yet been verified.
- Add a small store constructor that wraps a `pgxpool.Pool`.

```go
type Repository interface {
	CreateThought(ctx context.Context, params CreateThoughtParams) (Thought, error)
	GetThought(ctx context.Context, id uuid.UUID) (Thought, error)
	ListRecent(ctx context.Context, params ListRecentParams) ([]Thought, error)
	SearchKeyword(ctx context.Context, params SearchKeywordParams) ([]Thought, error)
	SearchSemantic(ctx context.Context, params SearchSemanticParams) ([]Thought, error)
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
git add openbrainfun/migrations/0001_initial.sql openbrainfun/internal/thoughts/types.go openbrainfun/internal/thoughts/repository.go openbrainfun/internal/thoughts/types_test.go openbrainfun/internal/postgres/store.go openbrainfun/internal/postgres/store_test.go
git commit -m "openbrainfun: add thought domain and schema"
```

### Task 3: Implement capture service with pending-ingest persistence

**Files:**
- Create: `openbrainfun/internal/thoughts/service.go`
- Create: `openbrainfun/internal/thoughts/service_test.go`
- Create: `openbrainfun/internal/http/handlers_capture.go`
- Create: `openbrainfun/internal/http/handlers_capture_test.go`
- Create: `openbrainfun/internal/http/router.go`

**Step 1: Write the failing capture service test**

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/thoughts ./internal/http -run 'TestCreateThoughtStoresPendingRecord|TestPostThoughtReturnsCreated' -v`

Expected: FAIL because the service and HTTP handler do not exist.

**Step 3: Write minimal implementation**

- Add `Service.CreateThought` that validates input and writes pending rows.
- Add `POST /api/thoughts` that accepts content, exposure scope, and user tags.
- Add `GET /healthz` to the router if not already wired.
- Return `201 Created` with JSON containing the new thought id and ingest status.

```go
type CreateThoughtInput struct {
	Content       string
	ExposureScope ExposureScope
	UserTags      []string
	Source        string
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/thoughts ./internal/http -v`

Expected: PASS

**Step 5: Commit**

```bash
git add openbrainfun/internal/thoughts/service.go openbrainfun/internal/thoughts/service_test.go openbrainfun/internal/http/handlers_capture.go openbrainfun/internal/http/handlers_capture_test.go openbrainfun/internal/http/router.go
git commit -m "openbrainfun: add pending thought capture path"
```

### Task 4: Add the Ollama client and background ingest worker

**Files:**
- Create: `openbrainfun/internal/ollama/client.go`
- Create: `openbrainfun/internal/ollama/client_test.go`
- Create: `openbrainfun/internal/worker/processor.go`
- Create: `openbrainfun/internal/worker/processor_test.go`
- Modify: `openbrainfun/cmd/openbrain/main.go`

**Step 1: Write the failing worker test**

```go
func TestProcessorMarksThoughtReadyAfterEmbedding(t *testing.T) {
	repo := &fakeRepo{pending: []Thought{{ID: mustUUID(t), Content: "remember to add retry visibility", IngestStatus: IngestPending}}}
	embedder := &fakeEmbedder{vector: []float32{0.1, 0.2, 0.3}}
	processor := NewProcessor(repo, embedder, nil)

	if err := processor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repo.readyCalls) != 1 {
		t.Fatalf("readyCalls = %d, want 1", len(repo.readyCalls))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/ollama ./internal/worker -v`

Expected: FAIL because the client and worker do not exist.

**Step 3: Write minimal implementation**

- Add an `Embedder` interface and Ollama implementation that calls `/api/embed`.
- Keep the initial client limited to embeddings; metadata extraction can be added after the retrieval slice works.
- Add `Processor.RunOnce` that claims pending rows, computes embeddings, and marks rows ready or failed.
- Start a polling goroutine from `main.go` using a ticker and context cancellation.

```go
type Embedder interface {
	Embed(ctx context.Context, input []string) ([][]float32, error)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/ollama ./internal/worker -v`

Expected: PASS

**Step 5: Commit**

```bash
git add openbrainfun/internal/ollama/client.go openbrainfun/internal/ollama/client_test.go openbrainfun/internal/worker/processor.go openbrainfun/internal/worker/processor_test.go openbrainfun/cmd/openbrain/main.go
git commit -m "openbrainfun: add ollama embedder and ingest worker"
```

### Task 5: Build the deterministic web UI for capture, browse, and search

**Files:**
- Create: `openbrainfun/internal/web/templates/layout.tmpl`
- Create: `openbrainfun/internal/web/templates/index.tmpl`
- Create: `openbrainfun/internal/web/templates/thoughts.tmpl`
- Create: `openbrainfun/internal/web/templates/search.tmpl`
- Create: `openbrainfun/internal/web/handlers.go`
- Create: `openbrainfun/internal/web/handlers_test.go`
- Modify: `openbrainfun/internal/http/router.go`

**Step 1: Write the failing web handler test**

```go
func TestIndexPageShowsCaptureForm(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h := NewHandlers(fakeQueryService{})
	h.Index(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "name="content"") {
		t.Fatalf("body missing capture form: %s", rr.Body.String())
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/web -v`

Expected: FAIL because the web handlers and templates do not exist.

**Step 3: Write minimal implementation**

- Add server-rendered templates.
- Implement `GET /`, `GET /thoughts`, and `GET /search`.
- Add keyword search and recent thoughts to the UI first.
- Add semantic search by calling the query service with an embedding computed for the search text.
- Surface `pending`, `ready`, and `failed` status visibly.

```go
type QueryService interface {
	ListRecent(ctx context.Context, limit int) ([]thoughts.Thought, error)
	SearchKeyword(ctx context.Context, query string, limit int) ([]thoughts.Thought, error)
	SearchSemantic(ctx context.Context, query string, limit int, viewer ViewerType) ([]thoughts.Thought, error)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/web -v`

Expected: PASS

**Step 5: Commit**

```bash
git add openbrainfun/internal/web/templates/layout.tmpl openbrainfun/internal/web/templates/index.tmpl openbrainfun/internal/web/templates/thoughts.tmpl openbrainfun/internal/web/templates/search.tmpl openbrainfun/internal/web/handlers.go openbrainfun/internal/web/handlers_test.go openbrainfun/internal/http/router.go
git commit -m "openbrainfun: add capture and search web ui"
```

### Task 6: Enforce exposure policy and MCP bearer authentication centrally

**Files:**
- Create: `openbrainfun/internal/auth/middleware.go`
- Create: `openbrainfun/internal/auth/middleware_test.go`
- Create: `openbrainfun/internal/query/service.go`
- Create: `openbrainfun/internal/query/service_test.go`
- Modify: `openbrainfun/internal/postgres/store.go`

**Step 1: Write the failing policy test**

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth ./internal/query -v`

Expected: FAIL because the query service and auth middleware do not exist.

**Step 3: Write minimal implementation**

- Add viewer-aware query service methods.
- Enforce `remote_ok` filtering for all remote MCP callers.
- Add bearer-auth middleware for `/mcp`.
- Keep UI routes separate; assume browser auth is enforced at the proxy.

```go
type ViewerType string

const (
	ViewerLocalUI   ViewerType = "local_ui"
	ViewerRemoteMCP ViewerType = "remote_mcp"
)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth ./internal/query -v`

Expected: PASS

**Step 5: Commit**

```bash
git add openbrainfun/internal/auth/middleware.go openbrainfun/internal/auth/middleware_test.go openbrainfun/internal/query/service.go openbrainfun/internal/query/service_test.go openbrainfun/internal/postgres/store.go
git commit -m "openbrainfun: enforce exposure policy for remote clients"
```

### Task 7: Expose a read-only Streamable HTTP MCP server

**Files:**
- Create: `openbrainfun/internal/mcp/server.go`
- Create: `openbrainfun/internal/mcp/server_test.go`
- Modify: `openbrainfun/internal/http/router.go`
- Modify: `openbrainfun/cmd/openbrain/main.go`

**Step 1: Write the failing MCP tool registration test**

```go
func TestServerRegistersReadOnlyTools(t *testing.T) {
	srv := NewServer(fakeQueryService{})
	tools := srv.ToolNames()
	want := []string{"search_thoughts", "recent_thoughts", "get_thought", "stats"}
	if diff := cmp.Diff(want, tools); diff != "" {
		t.Fatalf("ToolNames mismatch (-want +got):
%s", diff)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/mcp -v`

Expected: FAIL because the MCP server wrapper does not exist.

**Step 3: Write minimal implementation**

- Use `github.com/modelcontextprotocol/go-sdk/mcp`.
- Register four read-only tools.
- Mount the SDK’s Streamable HTTP transport at `/mcp`.
- Reuse the query service so policy and formatting logic stay centralized.
- Return concise text + structured JSON content where the SDK supports it.

```go
server := mcp.NewServer(&mcp.Implementation{
	Name:    "openbrain",
	Version: "v0.1.0",
}, nil)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/mcp -v`

Expected: PASS

**Step 5: Commit**

```bash
git add openbrainfun/internal/mcp/server.go openbrainfun/internal/mcp/server_test.go openbrainfun/internal/http/router.go openbrainfun/cmd/openbrain/main.go
git commit -m "openbrainfun: add read-only mcp server"
```

### Task 8: Add local operations scaffolding and manual verification docs

**Files:**
- Create: `openbrainfun/compose.yaml`
- Create: `openbrainfun/scripts/migrate.sh`
- Create: `openbrainfun/scripts/run-local.sh`
- Modify: `openbrainfun/README.md`

**Step 1: Write the failing smoke-check script expectation in the README**

Document these expected manual checks:

- `curl http://127.0.0.1:8080/healthz` returns `ok`
- posting a thought returns `201`
- a pending thought becomes ready after Ollama processes it
- MCP rejects missing bearer auth with `401`
- a `local_only` thought is absent from MCP results

**Step 2: Run the local stack commands before the scripts exist**

Run:

```bash
docker compose up -d postgres ollama
./scripts/migrate.sh
./scripts/run-local.sh
```

Expected: FAIL because the compose file and scripts do not exist.

**Step 3: Write minimal implementation**

- Add `compose.yaml` with Postgres and Ollama services.
- Add a migration script that applies SQL files in order.
- Add a run script that exports env vars and starts the Go binary.
- Update the README with Open WebUI configuration notes:
  - set `WEBUI_SECRET_KEY`
  - add an external tool of type `MCP (Streamable HTTP)`
  - point it at `https://<host>/mcp`
  - use bearer auth with the MCP token

**Step 4: Run the manual verification flow**

Run:

```bash
docker compose up -d postgres ollama
go test ./...
./scripts/migrate.sh
./scripts/run-local.sh
curl -i http://127.0.0.1:8080/healthz
```

Expected:

- `go test ./...` PASS
- health endpoint returns `HTTP/1.1 200 OK`
- local capture works
- Open WebUI can connect using Streamable HTTP

**Step 5: Commit**

```bash
git add openbrainfun/compose.yaml openbrainfun/scripts/migrate.sh openbrainfun/scripts/run-local.sh openbrainfun/README.md
git commit -m "openbrainfun: add local ops scaffolding"
```

### Task 9: Optional follow-up — metadata extraction after the vertical slice works

**Files:**
- Create: `openbrainfun/internal/metadata/extractor.go`
- Create: `openbrainfun/internal/metadata/extractor_test.go`
- Modify: `openbrainfun/internal/worker/processor.go`
- Modify: `openbrainfun/internal/web/templates/index.tmpl`
- Modify: `openbrainfun/internal/web/templates/thoughts.tmpl`

**Step 1: Write the failing extractor test**

```go
func TestNormalizeMetadataProvidesDefaultTopics(t *testing.T) {
	got := Normalize(map[string]any{})
	if len(got.Topics) == 0 {
		t.Fatal("expected at least one default topic")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/metadata -v`

Expected: FAIL because the extractor package does not exist.

**Step 3: Write minimal implementation**

- Add an Ollama chat client using JSON output.
- Normalize invalid/missing metadata into safe defaults.
- Update the worker to persist metadata when extraction succeeds and leave the thought otherwise searchable even if metadata extraction fails.
- Surface extracted topics in the UI, but keep manual tags first-class.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/metadata ./internal/worker -v`

Expected: PASS

**Step 5: Commit**

```bash
git add openbrainfun/internal/metadata/extractor.go openbrainfun/internal/metadata/extractor_test.go openbrainfun/internal/worker/processor.go openbrainfun/internal/web/templates/index.tmpl openbrainfun/internal/web/templates/thoughts.tmpl
git commit -m "openbrainfun: add optional metadata extraction"
```

## Final verification checklist

- `go test ./...`
- `docker compose up -d postgres ollama`
- apply migrations successfully
- create one `local_only` thought and verify it is absent from MCP search results
- create one `remote_ok` thought and verify it appears in MCP results
- stop Ollama, capture a thought, verify it stays `pending`
- restart Ollama, verify the worker marks it `ready`
- connect Open WebUI using MCP (Streamable HTTP) and bearer auth

## Delivery notes

- Do not add write-capable MCP tools in v1.
- Do not expose app routes directly on the public interface; keep them behind the reverse proxy.
- If the MCP Go SDK’s streamable transport proves incompatible with Open WebUI during the initial spike, stop and verify the transport behavior from the official SDK examples/docs before proceeding. A fallback path is to expose the same internal query service over OpenAPI and optionally place an MCP wrapper at the edge later.
