# Local Open Brain Design

## Summary

Build a local-first “open brain” service with a Go application, Postgres + pgvector storage, and Ollama embeddings. The product has two externally visible surfaces:

1. an authenticated browser experience for login, capture, browse, search, edit, and delete
2. a read-only MCP endpoint on a separate port, authenticated by bearer tokens that each map to exactly one user

The system is explicitly multi-user from the beginning. Thoughts belong to a user. Browser access is allowed only through username/password login. MCP access is allowed only through a mapped bearer token, and every MCP read/search is restricted to the mapped user’s `remote_ok` thoughts.

The design also treats the project README as part of the product contract. The README must include a walkthrough generated from real interactions that shows database provisioning, browser/API login via curl, thought creation and retrieval, and an MCP query returning the created thought.

## Goals

- Own the full data path locally while still supporting remote access.
- Ship a fully working authenticated product, not a loose prototype.
- Make every browser page require login except the login page itself.
- Keep user data isolated by default in both web and MCP paths.
- Support deterministic browse/search without needing a chat model.
- Support semantic search with a self-hosted embedding runtime.
- Make the UI work well on mobile and desktop.
- Make setup, storage locations, and walkthrough behavior explicit in the README.
- Keep deployment simple enough for one always-on host behind a reverse proxy.

## Non-goals for v1

- Self-service signup.
- Self-service password reset or password change.
- Admin UI for user provisioning.
- Shared/collaborative thoughts across users.
- Write-capable MCP tools.
- Multiple embedding providers active in one deployment at the same time.
- App-managed TLS certificates.

## Verified constraints and external dependencies

- Open WebUI natively supports MCP in Streamable HTTP mode and says MCP is supported starting in v0.6.31. It also notes that OpenAPI is preferred for many deployments, but MCP is appropriate when MCP-native interoperability is required. Source: <https://docs.openwebui.com/features/mcp>
- Open WebUI’s MCP docs say native support is Streamable HTTP only, not stdio or SSE. Source: <https://docs.openwebui.com/features/mcp>
- Ollama officially supports local embeddings, recommends `embeddinggemma`, `qwen3-embedding`, and `all-minilm`, and exposes `/api/embed` plus OpenAI-compatible `/v1/embeddings`. Sources: <https://docs.ollama.com/capabilities/embeddings>, <https://docs.ollama.com/api/embed>, <https://docs.ollama.com/api/openai-compatibility>
- The Ollama library page for `all-minilm` showed a 46 MB model on March 14, 2026, making it a good default for quickstarts and CI. Source: <https://ollama.com/library/all-minilm>
- The Ollama library pages for `embeddinggemma` and `qwen3-embedding:0.6b` provide higher-capacity alternatives if an operator wants to trade more disk/RAM for quality. Sources: <https://ollama.com/library/embeddinggemma>, <https://ollama.com/library/qwen3-embedding:0.6b>
- pgvector officially supports exact and approximate nearest-neighbor search in Postgres and supports HNSW indexes. Source: <https://github.com/pgvector/pgvector>
- The official MCP Go SDK exists and the release notes show Streamable HTTP JSON response support, which is useful for curl-friendly walkthroughs and test automation. Source: <https://github.com/modelcontextprotocol/go-sdk/releases>

## Product requirements

- Users are manually provisioned in the database for v1.
- Browser auth is login/logout only.
- Every thought belongs to exactly one user.
- In the web UI, users can only read, search, edit, and delete their own thoughts.
- In MCP, a bearer token maps to exactly one user.
- MCP callers can only read/search that mapped user’s thoughts, and only when `exposure_scope='remote_ok'`.
- The browser UI must be fully usable without JavaScript.
- The browser UI must have visible submit/save/delete controls; it cannot rely on implied or hidden affordances.
- The README walkthrough must be produced from real interactions and verified in CI.

## Recommended architecture

### Edge

Use a reverse proxy for:

- public DNS and TLS termination
- routing browser traffic to the web listener
- routing MCP traffic to the MCP listener on a separate port
- rate limiting and request logging

Do not add app-level TLS in v1. Bind the Go listeners to private interfaces or localhost behind the proxy.

### Application

Run one Go binary with two listeners:

1. **web listener** — login/logout, server-rendered UI, and a small authenticated JSON API used by the UI, tests, and README walkthrough
2. **MCP listener** — read-only Streamable HTTP MCP surface using bearer-token auth

Run one internal background worker loop in the same process for v1. This keeps deployment simple while still allowing asynchronous embedding work.

### Data

Use Postgres as the source of truth.

Why Postgres instead of SQLite:

- better fit for authenticated multi-device access
- first-class pgvector support
- safer concurrent reads/writes from UI + MCP + worker
- straightforward server-side ownership filters by `user_id`
- simpler long-term path for backup and schema evolution

### Model runtime

Use Ollama first.

Reasoning:

- official embedding docs are clear and current
- the `/api/embed` API is simple and local-first
- `all-minilm` makes quickstarts and CI practical on CPU-only machines
- stronger alternative models remain available by configuration later

## Core domain model

Use four primary tables in v1 plus pgvector/pgcrypto extensions.

Suggested shape:

```sql
create extension if not exists vector;
create extension if not exists pgcrypto;

create type exposure_scope as enum ('local_only', 'remote_ok');
create type ingest_status as enum ('pending', 'processing', 'ready', 'failed');

create table users (
  id uuid primary key default gen_random_uuid(),
  username text not null unique,
  password_hash text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  disabled_at timestamptz
);

create table web_sessions (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  session_token_hash text not null unique,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now()
);

create table mcp_tokens (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  label text not null,
  token_hash text not null unique,
  created_at timestamptz not null default now(),
  last_used_at timestamptz,
  revoked_at timestamptz
);

create table thoughts (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  content text not null check (btrim(content) <> ''),
  source text not null default 'web',
  exposure_scope exposure_scope not null default 'local_only',
  user_tags text[] not null default '{}',
  metadata jsonb not null default '{}'::jsonb,
  embedding vector(__EMBED_DIMENSIONS__),
  embedding_model text,
  ingest_status ingest_status not null default 'pending',
  ingest_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index web_sessions_user_id_idx on web_sessions (user_id);
create index mcp_tokens_user_id_idx on mcp_tokens (user_id);
create index thoughts_user_id_created_at_idx on thoughts (user_id, created_at desc);
create index thoughts_user_id_exposure_scope_idx on thoughts (user_id, exposure_scope);
create index thoughts_user_tags_gin_idx on thoughts using gin (user_tags);
create index thoughts_metadata_gin_idx on thoughts using gin (metadata);
create index thoughts_embedding_hnsw_idx
  on thoughts using hnsw (embedding vector_cosine_ops)
  where ingest_status = 'ready' and embedding is not null;
```

Notes:

- `user_id` ownership is mandatory on all read/write paths.
- `__EMBED_DIMENSIONS__` is intentionally a placeholder. The migration helper should probe the configured Ollama model first and render the concrete dimension before applying the migration.
- Only one embedding model is active per deployment at a time. Changing models later requires a documented re-embed + reindex workflow.
- Deletion is a hard delete in v1 so “delete” means the thought is gone.

## Authentication and trust model

### Web auth

- `/login` is the only public browser page.
- Successful login creates a server-side session and sets a secure cookie.
- Logout invalidates the session and clears the cookie.
- Every page under `/`, `/thoughts`, and `/api/*` requires a valid session.
- Browser write actions must use CSRF protection.

### MCP auth

- MCP runs on a separate port.
- Every request requires a bearer token.
- Each token maps to exactly one user.
- MCP never uses browser sessions.
- MCP is read-only in v1.

### Trust boundaries

- Browser requests are scoped by authenticated session user.
- MCP requests are scoped by token-mapped user.
- `remote_ok` is an additional MCP filter, not a substitute for `user_id` ownership.
- Logs must avoid raw passwords, raw bearer tokens, and full thought contents by default.

## Web UI contract

Keep the UI server-rendered and boring, but fully formed.

### General UI rules

- Every form has a visible primary action button.
- Core flows work without JavaScript.
- Mobile layout must avoid horizontal scrolling for the main experience.
- Desktop layout should use more width while keeping forms readable.
- Navigation and actions must remain visible on both mobile and desktop.
- Do not hide required actions behind hover-only controls.

### `/login`

Required elements:

- username field
- password field
- visible **Log in** button
- generic inline error on invalid credentials

Behavior:

- successful login redirects to `/`
- authenticated users visiting `/login` are redirected to `/`

### `/`

Authenticated dashboard page.

Required elements:

- top navigation with Home, Thoughts, Log out
- capture form with:
  - content textarea
  - exposure scope control
  - tags input
  - visible **Save thought** button
- recent-thoughts list below the form
- pending/ready/failed ingest status visible for recent thoughts
- success/error flash messages after form actions

Behavior:

- POST/Redirect/GET after successful create
- invalid form submissions re-render with entered values and validation errors

### `/thoughts`

Primary browse/manage page.

Required elements:

- keyword search input
- filters for exposure scope and ingest status
- list of only the current user’s thoughts
- each row/card shows excerpt, tags, ingest status, updated time, and an **Edit** action

Behavior:

- mobile layout uses stacked cards
- desktop layout may use a table or spacious list
- search and filter state stays visible in the URL

### `/thoughts/{id}`

Thought detail/edit page.

Required elements:

- full editable content
- editable tags
- editable exposure scope
- visible **Save changes** button
- visible **Delete thought** button
- delete confirmation step
- ingest status and ingest error display

Behavior:

- only the owning user may access the page
- successful edit uses POST/Redirect/GET
- editing searchable fields resets ingest to `pending`
- delete permanently removes the thought after confirmation

## JSON API contract

The web listener also exposes a small authenticated JSON API used for curl walkthroughs, tests, and automation.

Required endpoints:

- `POST /api/session` — login, sets session cookie
- `DELETE /api/session` — logout
- `POST /api/thoughts` — create a thought for the current user
- `GET /api/thoughts/{id}` — fetch one thought owned by the current user
- `PATCH /api/thoughts/{id}` — edit one thought owned by the current user
- `DELETE /api/thoughts/{id}` — delete one thought owned by the current user
- `GET /api/thoughts` — list/search the current user’s thoughts

Rules:

- the API uses the same ownership checks as the server-rendered UI
- JSON endpoints return structured errors and appropriate status codes
- the README walkthrough should use these endpoints rather than scraping HTML

## Request and data flows

### Login flow

1. User submits username and password.
2. App validates the account and password hash.
3. App stores a server-side session row and sets a session cookie.
4. Browser is redirected to `/`.

### Capture flow

1. Authenticated browser or curl client submits content, exposure scope, and optional tags.
2. API validates ownership context and input.
3. App stores a thought row with `ingest_status='pending'`.
4. App returns success immediately.
5. Background worker claims pending thoughts in batches.
6. Worker requests embeddings from Ollama.
7. Worker updates the thought to `ready` or `failed`.
8. UI and API show pending/ready/failed state.

### Edit flow

1. User loads `/thoughts/{id}`.
2. User edits content, tags, and/or exposure scope.
3. App validates ownership and input.
4. If searchable fields changed, app stores the edit and resets ingest to `pending`.
5. Worker re-embeds the thought.

### Delete flow

1. User requests delete from `/thoughts/{id}`.
2. App shows an explicit confirmation step.
3. Confirmed delete permanently removes the row.

### Search flow

- Keyword search always works over the current user’s thoughts.
- Semantic search only includes the current user’s `ready` thoughts.
- MCP search applies both `user_id` scoping and `remote_ok` filtering.

## MCP tool surface

Keep the MCP surface deliberately small in v1:

- `search_thoughts` — semantic and/or keyword retrieval over the mapped user’s `remote_ok` thoughts
- `recent_thoughts` — browse the mapped user’s recent `remote_ok` thoughts
- `get_thought` — fetch one mapped-user `remote_ok` thought by id
- `stats` — count totals for the mapped user, with MCP-visible counts scoped to `remote_ok`

Do not expose write tools in v1.

## Deployment and persistence

Single-host deployment:

- reverse proxy
- one Go application binary with web + MCP listeners
- Postgres with pgvector
- Ollama

For local development and README walkthroughs, use Docker Compose with explicit bind mounts:

- `./var/postgres` for Postgres data
- `./var/ollama` for Ollama model cache

These paths must be gitignored and described in the README so operators know exactly where data lives on disk.

## README and walkthrough contract

The README is a required project artifact, not an afterthought.

It must contain:

1. a brief architecture overview
2. startup instructions for local development
3. explicit notes about persistent storage locations
4. exact setup for the embedding model used in the walkthrough
5. a walkthrough generated from real interactions that shows:
   - starting the stack
   - provisioning a demo user in Postgres
   - provisioning a demo MCP token mapped to that user
   - logging in with curl and storing a cookie jar
   - creating a thought with curl
   - retrieving that thought with curl
   - issuing an MCP query with curl that returns the thought

Implementation expectations:

- the walkthrough should be generated from a script, not maintained by hand
- CI should run the script, keep the generated transcript as an artifact, and fail if the checked-in README walkthrough is out of sync
- the walkthrough should use the same app paths and auth model that the product actually uses

## Embedding model strategy

Use one configured Ollama embedding model per deployment.

### v1 defaults

- README quickstart: `all-minilm:22m`
- real-embed CI job: `all-minilm:22m`

Reasoning:

- it is officially recommended by Ollama for embeddings
- it is lightweight enough for CPU-only quickstarts and CI
- the Ollama library page showed a 46 MB model on March 14, 2026

### Supported operator override

Allow operators to configure another Ollama embedding model, such as:

- `embeddinggemma`
- `qwen3-embedding:0.6b`

Rules:

- the app must expose the selected model via configuration
- the migration/bootstrap path must verify model dimension before creating or accepting the schema
- startup must fail fast if the configured model and schema dimension do not match

## Failure handling

- Invalid login returns a generic credential error without username enumeration.
- Expired or invalid sessions redirect to login for browser requests and return auth errors for API requests.
- Invalid or revoked MCP tokens return typed auth errors.
- If Ollama is unavailable, create/edit requests still succeed and leave thoughts `pending`.
- If embedding fails repeatedly, the thought is marked `failed` with a visible error and a retry path.
- Only `ready` thoughts participate in semantic search.
- Keyword search remains available even when embeddings are unavailable.
- Delete on a missing or non-owned thought returns not-found rather than leaking ownership details.
- Concurrent edits may use last-write-wins in v1 if clearly documented.

## Testing strategy and quality gates

### Quality gates

- Code must be `gofmt` clean.
- Code must pass `go vet`.
- Automated coverage must stay very high.
- The design target is at least 90% repo-wide coverage and at least 95% for core packages that enforce auth, ownership, CRUD, and MCP filtering.

### Automated tests

- unit tests for config parsing, session auth, token auth, ownership filtering, capture validation, edit/delete flows, worker retry behavior, and MCP handlers
- repository tests for SQL query shape and user scoping
- template/handler tests that prove visible submit/save/delete controls exist on rendered pages
- integration tests for login → create → edit → delete → search using the fake embedder
- shared contract tests for the embedder interface run against both the fake implementation and the real Ollama implementation
- CI-backed tests against a real Ollama process using `all-minilm:22m`
- CI verification that one user cannot read another user’s thoughts through web, API, or MCP
- CI verification that the README walkthrough still matches real behavior

### Test backend split

Use an embedder interface with two implementations:

1. **fake embedder** for deterministic unit and most integration tests
2. **Ollama embedder** for contract tests and selected real integration tests

The same backend-agnostic contract cases should run against both implementations so fake-backend assumptions are verified against the real dependency.

## Deferred decisions

- whether to add automatic metadata extraction in the first milestone or after CRUD/auth/search works end to end
- whether to add an admin CLI for provisioning users/tokens instead of raw SQL walkthrough steps
- whether to expose a parallel OpenAPI-only search endpoint for easier third-party debugging
- whether to split the worker into a second process after the system proves useful
