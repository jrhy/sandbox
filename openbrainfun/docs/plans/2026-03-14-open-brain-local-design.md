# Local Open Brain Design

## Summary

Build a local-first “open brain” service that replaces the Supabase/OpenRouter stack with a self-hosted Go application, Postgres + pgvector, and Ollama for embeddings (and optionally metadata extraction). The system will expose a web UI for capture and deterministic browse/search, plus a read-only public MCP endpoint for external LLM clients such as Open WebUI.

## Goals

- Own the full data path locally while still supporting public remote access.
- Keep capture deterministic and fast.
- Support semantic search with self-hosted embeddings.
- Preserve a non-LLM browse/search path for debugging and trust.
- Enforce privacy policy on the server side, not in clients.
- Keep the architecture simple enough to run on one always-on host behind a reverse proxy.

## Non-goals for v1

- Multi-user collaboration.
- Fine-grained vendor-specific ACLs.
- Local Slack ingestion.
- Full-text ranking or advanced relevance tuning.
- Multiple embedding providers.
- App-managed TLS certificates.

## Verified constraints and external dependencies

- The PromptKit guide explicitly chooses hosted infrastructure for convenience and notes that local MCP is reasonable if you want to customize the system. Source: <https://promptkit.natebjones.com/20260224_uq1_guide_main>
- Open WebUI natively supports MCP in “Streamable HTTP” mode and says MCP is supported starting in v0.6.31. It also notes that OpenAPI is preferred for most deployments, but MCP is appropriate when you need MCP-native interoperability. Source: <https://docs.openwebui.com/features/mcp>
- Open WebUI’s MCP docs say native support is Streamable HTTP only, not stdio or SSE. Source: <https://docs.openwebui.com/features/mcp>
- Ollama officially supports local embeddings, recommends embeddinggemma / qwen3-embedding / all-minilm, and exposes `/api/embed` plus OpenAI-compatible `/v1/embeddings`. Sources: <https://docs.ollama.com/capabilities/embeddings>, <https://docs.ollama.com/api/embed>, <https://docs.ollama.com/api/openai-compatibility>
- pgvector officially supports exact and approximate nearest-neighbor search in Postgres and supports HNSW indexes. Source: <https://github.com/pgvector/pgvector>
- The official MCP Go SDK exists and is maintained by modelcontextprotocol. The current GitHub page I checked on March 14, 2026 shows release v1.4.1 dated March 13, 2026. Source: <https://github.com/modelcontextprotocol/go-sdk>

## Recommended architecture

### Edge

Use a reverse proxy for:

- public DNS and TLS termination
- routing `/` and `/api/*` to the web app
- routing `/mcp` to the MCP transport endpoint
- optional UI auth at the edge for browser access
- rate limiting and request logging

Do not add app-level TLS in v1. Bind the Go service to a private interface or localhost behind the proxy.

### Application

Run one Go service with three responsibilities:

1. web UI for capture, browse, and manual review
2. JSON API used by the web UI
3. public read-only MCP server over Streamable HTTP

Run one background worker loop inside the same process for v1. This keeps deployment simple while still allowing asynchronous embedding work.

### Data

Use Postgres as the source of truth.

Why Postgres instead of SQLite:

- better fit for public multi-device access
- first-class pgvector support
- safer concurrent reads/writes from UI + MCP + worker
- simpler long-term path for backup, indexing, and schema evolution

### Model runtime

Use Ollama first.

Reasoning:

- official embedding docs are clear and current
- OpenAI-compatible endpoints reduce custom glue
- lower implementation risk than an MLX-first path
- still leaves room to revisit MLX later if a strong embedding-serving path becomes attractive

## Core domain model

Use one `thoughts` table in v1.

Suggested shape:

```sql
create extension if not exists vector;
create extension if not exists pgcrypto;

create type exposure_scope as enum ('local_only', 'remote_ok');
create type ingest_status as enum ('pending', 'processing', 'ready', 'failed');

create table thoughts (
  id uuid primary key default gen_random_uuid(),
  content text not null check (btrim(content) <> ''),
  source text not null default 'web',
  exposure_scope exposure_scope not null default 'local_only',
  allowed_client_tags text[] not null default '{}',
  user_tags text[] not null default '{}',
  metadata jsonb not null default '{}'::jsonb,
  embedding vector(768),
  embedding_model text,
  ingest_status ingest_status not null default 'pending',
  ingest_error text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index thoughts_created_at_idx on thoughts (created_at desc);
create index thoughts_metadata_gin_idx on thoughts using gin (metadata);
create index thoughts_user_tags_gin_idx on thoughts using gin (user_tags);
create index thoughts_embedding_hnsw_idx
  on thoughts using hnsw (embedding vector_cosine_ops)
  where ingest_status = 'ready' and embedding is not null;
```

Notes:

- `exposure_scope` is the v1 enforcement switch.
- `allowed_client_tags` is the escape hatch for future finer-grained trust policies.
- `user_tags` gives deterministic non-LLM categorization even if metadata extraction is deferred or wrong.
- `embedding` dimension must match the chosen embedding model. The initial DDL should be created only after the model choice is confirmed with a quick probe.

## Privacy and trust model

Server-side policy is mandatory.

### v1 policy

- Web UI can access every thought.
- Public MCP can only access `remote_ok` thoughts.
- Capture is web-only.
- MCP is read-only.

### Future policy path

Keep `allowed_client_tags` in the schema, but do not implement client-specific ACL logic in v1. That lets the system evolve toward:

- local-only
- trusted-local
- remote-safe
- specific client tags

without forcing vendor-specific logic into today’s design.

## Request/data flow

### Capture flow

1. Browser submits a thought with content, exposure scope, and optional tags.
2. API validates input and stores a row with `ingest_status='pending'`.
3. API returns success immediately.
4. Background worker fetches pending thoughts in batches.
5. Worker requests embeddings from Ollama.
6. Worker optionally requests structured metadata extraction from a chat model via Ollama.
7. Worker updates the row to `ready` or `failed`.
8. Web UI shows pending/ready/failed state.

### Non-LLM browse/search flow

The web UI exposes:

- recent thoughts
- keyword search on raw content and tags
- filters by exposure scope / ingest status / tags
- semantic search using vector similarity

This path exists so retrieval quality and policy enforcement can be verified without involving a chat model.

### MCP flow

1. MCP client connects to `/mcp` over Streamable HTTP.
2. Bearer token auth is validated by the app.
3. MCP tools call the same internal search/query services used by the web UI.
4. Query service always applies `remote_ok` filtering before returning results.

## MCP tool surface

Keep the MCP surface deliberately small in v1:

- `search_thoughts` — semantic and/or keyword retrieval over remote-safe thoughts
- `recent_thoughts` — browse recent remote-safe thoughts
- `get_thought` — fetch one remote-safe thought by id
- `stats` — count totals by ingest state / exposure scope (remote-safe only for public callers)

Do not expose write tools in v1.

## Web UI scope

Keep the UI server-rendered and boring.

Recommended pages:

- `/` capture form + recent thoughts
- `/thoughts` list with filters
- `/thoughts/{id}` detail / edit metadata and exposure
- `/search` keyword + semantic search results

Prefer standard `html/template` plus minimal JavaScript. Avoid a separate SPA.

## Failure handling

- If Ollama is unavailable, capture still succeeds and stores `pending` rows.
- If embedding fails repeatedly, mark the row `failed` with a visible error string and expose a retry action in the UI.
- Only `ready` rows participate in semantic search.
- Keyword search should continue to work for all rows.
- MCP tools should return typed, user-readable errors instead of silent empty results for auth/config failures.

## Security model

- Reverse proxy terminates TLS.
- Browser auth can be handled at the edge in v1.
- MCP uses app-enforced bearer auth.
- Go service listens only on private network interfaces.
- All public MCP responses are filtered by exposure policy in the application layer.
- Logs must avoid dumping full thought content unless debug logging is explicitly enabled.

## Deployment shape

Single-host deployment:

- reverse proxy
- Go service
- Postgres with pgvector
- Ollama

NAT is acceptable. The proxy host only needs a stable public route to the service host.

## Testing strategy

### Automated

- unit tests for config parsing, auth middleware, visibility filtering, capture validation, worker retry behavior, and MCP tool handlers
- repository tests for SQL query shape and policy filters
- integration tests for end-to-end capture → pending → ready using fakes for the embedder

### Manual

- create a `local_only` thought and verify it appears in the UI but not via MCP
- create a `remote_ok` thought and verify it appears in both UI and MCP
- stop Ollama and verify capture still works while new rows remain `pending`
- restart Ollama and verify retry completes
- connect Open WebUI to the MCP endpoint in Streamable HTTP mode

## Deferred decisions

- Whether to add automatic metadata extraction in the first milestone or after end-to-end capture/search works
- Whether to expose a parallel OpenAPI read-only search endpoint for easier debugging and as a fallback if some MCP clients are brittle
- Whether to migrate to an MLX-based embedding runtime later
- Whether to split the worker into a second process after the system proves useful
