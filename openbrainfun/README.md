# OpenBrainFun

OpenBrainFun is a local-first, multi-user “open brain” system built in Go.

It is directly inspired by Nate B Jones’ Open Brain guide:

- <https://promptkit.natebjones.com/20260224_uq1_guide_main>

The goal is similar: keep thoughts in a store that can be searched and reused
through MCP by external AI clients. The main differences are intentional:

- it is self-hosted and local-first rather than built around hosted services
- it replaces Supabase with Postgres + pgvector
- it replaces Slack capture with a built-in web UI and JSON API
- it replaces remote embedding dependencies with Ollama running locally
- it adds DB-backed username/password sessions for the browser UI
- it scopes thoughts and MCP access per user from day one
- it treats the README walkthrough, local operations docs, and verification
  scripts as part of the project contract

In practice, OpenBrainFun provides:

- a password-protected browser UI for capturing and editing thoughts
- a separate MCP endpoint on its own port, authenticated by bearer token
- Postgres + pgvector storage
- Ollama-based local embeddings and best-effort metadata extraction

## What runs

- web UI / JSON API on `OPENBRAIN_WEB_ADDR` (default walkthrough address: `127.0.0.1:18080`)
- MCP on `OPENBRAIN_MCP_ADDR` (default walkthrough address: `127.0.0.1:18081`)
- PostgreSQL via `compose.yaml`
- Ollama via `compose.yaml`

## Persistent data

Local persistent data is stored in bind mounts so it is obvious where state
lives:

- `./var/postgres`
- `./var/ollama`

## Models used in practice

- embedding model: `all-minilm:22m`
- metadata model: `qwen3:0.6b`

Both are configurable:

- `OPENBRAIN_EMBED_MODEL`
- `OPENBRAIN_METADATA_MODEL`

## Quick start

```bash
docker compose up -d postgres ollama
./scripts/walkthrough.sh
```

The walkthrough script provisions a demo user through the `openbrain` CLI,
starts the app with `openbrain start`, captures real HTTP interactions, and
updates both `docs/walkthrough.demo.md` and the generated walkthrough section
below.

## CLI

Running `go run ./cmd/openbrain` with no subcommand prints usage instead of
trying to boot the server.

Available commands:

- `go run ./cmd/openbrain start`
- `go run ./cmd/openbrain user update <username> --password <password>`
- `go run ./cmd/openbrain user delete <username>`
- `go run ./cmd/openbrain token create <username> [--label <label>]`
- `go run ./cmd/openbrain token list <username>`
- `go run ./cmd/openbrain token delete <username> --label <label>`

`user update` creates a default MCP token only when the user does not already
have one, and prints that token once. Token plaintext is not recoverable later;
use `token create` to mint or rotate a token for an existing user.

## Operations

See [docs/operations.md](docs/operations.md) for backup/restore notes and the
recommended re-embedding flow when models change.

<!-- walkthrough:start -->

## Walkthrough

This section is generated from `./scripts/walkthrough.sh`.

### Persistent data

Persistent data lives in:
- `./var/postgres`
- `./var/ollama`

### Models used in this walkthrough

- Embedding model: `all-minilm:22m`
- Metadata model: `qwen3:0.6b`

_Generated at 2026-03-15T18:13:04Z._

### Create or update the demo user

```bash
go run ./cmd/openbrain user update 'demo' --password 'demo-password' --token-label 'default'
```

```text
updated user username=demo
created default token label=default
token=pJUqc8qZ98y9ampABZ46kRLV5L5hvX4buLFqxDxH9Uk
note: this token will not be shown again; use `openbrain token create demo --label default` to rotate it
```

### Log in and receive a CSRF token

```bash
curl -sS -i -c '/var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.dcBrbMwtvK' -H 'Content-Type: application/json' -X POST 'http://127.0.0.1:18080/api/session' --data '{"username":"demo","password":"demo-password"}'
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Set-Cookie: openbrain_session=cBN7-6Pb8bo9Z7RDDQUYp6TtPiNVL13BEhTEVqHiaJA; Path=/; Expires=Mon, 16 Mar 2026 18:12:47 GMT; HttpOnly; SameSite=Lax
Date: Sun, 15 Mar 2026 18:12:47 GMT
Content-Length: 82

{"csrf_token":"5e28298dc9dfbaea92c2f1b1d4cece984653176ca4043471c9362ae717b40f5c"}
```

### Create a thought

```bash
curl -sS -i -b '/var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.dcBrbMwtvK' -H 'Content-Type: application/json' -H 'X-CSRF-Token: <csrf-token>' -X POST 'http://127.0.0.1:18080/api/thoughts' --data '{"content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","user_tags":["mcp","sessions"]}'
```

```text
HTTP/1.1 201 Created
Content-Type: application/json
Date: Sun, 15 Mar 2026 18:12:47 GMT
Content-Length: 326

{"id":"fa8cfe88-460a-4725-8058-a8c3885ab67d","content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","user_tags":["mcp","sessions"],"metadata":{"Summary":"No summary available.","Topics":[],"Entities":[]},"ingest_status":"pending","created_at":"2026-03-15T18:12:47Z","updated_at":"2026-03-15T18:12:47Z"}
```

### Retrieve the thought after background processing

```bash
curl -sS -i -b '/var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.dcBrbMwtvK' 'http://127.0.0.1:18080/api/thoughts/fa8cfe88-460a-4725-8058-a8c3885ab67d'
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 15 Mar 2026 18:13:04 GMT
Content-Length: 561

{"id":"fa8cfe88-460a-4725-8058-a8c3885ab67d","content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","user_tags":["mcp","sessions"],"metadata":{"Summary":"Remember to manage authentication through MCP and ensure local sessions are properly handled for seamless user experience.","Topics":["MCP authentication","local session management","authentication process"],"Entities":["MCP","auth","local sessions"]},"embedding_model":"all-minilm:22m","ingest_status":"ready","created_at":"2026-03-15T18:12:47Z","updated_at":"2026-03-15T18:13:03Z"}
```

### Query MCP

```bash
curl -sS -i -H 'Content-Type: application/json' -H 'Authorization: Bearer pJUqc8qZ98y9ampABZ46kRLV5L5hvX4buLFqxDxH9Uk' -X POST 'http://127.0.0.1:18081/mcp' --data '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_thoughts","arguments":{"query":"MCP auth"}}}'
```

```text
HTTP/1.1 200 OK
Cache-Control: no-cache, no-transform
Content-Type: application/json
Date: Sun, 15 Mar 2026 18:13:04 GMT
Content-Length: 504

{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"thoughts\":[{\"content\":\"Remember MCP auth and local sessions\",\"exposure_scope\":\"remote_ok\",\"id\":\"fa8cfe88-460a-4725-8058-a8c3885ab67d\",\"ingest_status\":\"ready\",\"user_tags\":[\"mcp\",\"sessions\"]}]}"}],"structuredContent":{"thoughts":[{"content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","id":"fa8cfe88-460a-4725-8058-a8c3885ab67d","ingest_status":"ready","user_tags":["mcp","sessions"]}]}}}
```

<!-- walkthrough:end -->
