# OpenBrainFun

OpenBrainFun is a local-first, multi-user “open brain” system built in Go.

It provides:

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

The walkthrough script provisions a demo user and MCP token directly in the
database, starts the app, captures real HTTP interactions, and updates both
`docs/walkthrough.demo.md` and the generated walkthrough section below.

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

_Generated at 2026-03-15T17:28:02Z._

### Provision demo user and MCP token

```bash
./scripts/provision-demo-data.sh
```

```text
INSERT 0 1
DELETE 0
INSERT 0 1
provisioned username=demo token=demo-mcp-token label=demo
```

### Log in and receive a CSRF token

```bash
curl -sS -i -c '/var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.qrmS5hQNUE' -H 'Content-Type: application/json' -X POST 'http://127.0.0.1:18080/api/session' --data '{"username":"demo","password":"demo-password"}'
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Set-Cookie: openbrain_session=A8p8hagOI13ux7mV-15uH-YEsQkDB1OkFc7EPH65HTo; Path=/; Expires=Mon, 16 Mar 2026 17:27:46 GMT; HttpOnly; SameSite=Lax
Date: Sun, 15 Mar 2026 17:27:46 GMT
Content-Length: 82

{"csrf_token":"d5109a2db7c1464249b38af120e88d6a3cbcbd06939685847cb8d6657e2b4fce"}
```

### Create a thought

```bash
curl -sS -i -b '/var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.qrmS5hQNUE' -H 'Content-Type: application/json' -H 'X-CSRF-Token: <csrf-token>' -X POST 'http://127.0.0.1:18080/api/thoughts' --data '{"content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","user_tags":["mcp","sessions"]}'
```

```text
HTTP/1.1 201 Created
Content-Type: application/json
Date: Sun, 15 Mar 2026 17:27:46 GMT
Content-Length: 326

{"id":"63bdeef1-6171-4149-a849-6e86be30e5d8","content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","user_tags":["mcp","sessions"],"metadata":{"Summary":"No summary available.","Topics":[],"Entities":[]},"ingest_status":"pending","created_at":"2026-03-15T17:27:46Z","updated_at":"2026-03-15T17:27:46Z"}
```

### Retrieve the thought after background processing

```bash
curl -sS -i -b '/var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.qrmS5hQNUE' 'http://127.0.0.1:18080/api/thoughts/63bdeef1-6171-4149-a849-6e86be30e5d8'
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 15 Mar 2026 17:28:02 GMT
Content-Length: 503

{"id":"63bdeef1-6171-4149-a849-6e86be30e5d8","content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","user_tags":["mcp","sessions"],"metadata":{"Summary":"Concise summary: MCP authentication and local session management are essential for secure user sessions.","Topics":["MCP authentication","local sessions"],"Entities":["MCP","local","sessions"]},"embedding_model":"all-minilm:22m","ingest_status":"ready","created_at":"2026-03-15T17:27:46Z","updated_at":"2026-03-15T17:28:01Z"}
```

### Query MCP

```bash
curl -sS -i -H 'Content-Type: application/json' -H 'Authorization: Bearer demo-mcp-token' -X POST 'http://127.0.0.1:18081/mcp' --data '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_thoughts","arguments":{"query":"MCP auth"}}}'
```

```text
HTTP/1.1 200 OK
Cache-Control: no-cache, no-transform
Content-Type: application/json
Date: Sun, 15 Mar 2026 17:28:02 GMT
Content-Length: 504

{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"thoughts\":[{\"content\":\"Remember MCP auth and local sessions\",\"exposure_scope\":\"remote_ok\",\"id\":\"63bdeef1-6171-4149-a849-6e86be30e5d8\",\"ingest_status\":\"ready\",\"user_tags\":[\"mcp\",\"sessions\"]}]}"}],"structuredContent":{"thoughts":[{"content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","id":"63bdeef1-6171-4149-a849-6e86be30e5d8","ingest_status":"ready","user_tags":["mcp","sessions"]}]}}}
```

<!-- walkthrough:end -->
