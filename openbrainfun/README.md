# OpenBrainFun

Local-first open-brain service with Go + Postgres/pgvector + Ollama.

## Manual checks

- `curl http://127.0.0.1:8080/healthz` returns `ok`
- posting a thought returns `201`
- a pending thought becomes ready after Ollama processes it
- MCP rejects missing bearer auth with `401`
- a `local_only` thought is absent from MCP results

## Provisioning: MCP bearer token

Set a token before starting the app. This token is required for `/mcp`.

```bash
export OPENBRAIN_MCP_BEARER_TOKEN=dev-token
```

If you run with Docker/Compose or a process manager later, set this as an environment variable or secret in that runtime.

## Local run

```bash
docker compose up -d postgres ollama
./scripts/migrate.sh
./scripts/run-local.sh
```

## Walkthrough with command output

The reproducible command log lives at:

- `docs/walkthrough.demo.md`

It was generated with Showboat.

### 0) Showboat help (required tool)

Command:

```bash
uvx showboat --help | sed -n '1,16p'
```

Output:

```text
showboat - Create executable demo documents that show and prove an agent's work.

Showboat helps agents build markdown documents that mix commentary, executable
code blocks, and captured output. These documents serve as both readable
documentation and reproducible proof of work. A verifier can re-execute all
code blocks and confirm the outputs still match.

Usage:
  showboat init <file> <title>             Create a new demo document
  showboat note <file> [text]              Append commentary (text or stdin)
  showboat exec <file> <lang> [code]       Run code and capture output
  showboat image <file> <path>             Copy image into document
  showboat image <file> '![alt](path)'   Copy image with alt text
  showboat pop <file>                      Remove the most recent entry
  showboat verify <file> [--output <new>]  Re-run and diff all code blocks
  showboat extract <file> [--filename <name>]  Emit commands to recreate file
```

### 1) Provision token

Command:

```bash
export OPENBRAIN_MCP_BEARER_TOKEN=dev-token
echo OPENBRAIN_MCP_BEARER_TOKEN=$OPENBRAIN_MCP_BEARER_TOKEN
```

Output:

```text
OPENBRAIN_MCP_BEARER_TOKEN=dev-token
```

### 2) Health check against running app

Command:

```bash
OPENBRAIN_DATABASE_URL=postgres://x \
OPENBRAIN_OLLAMA_URL=http://127.0.0.1:11434 \
OPENBRAIN_EMBED_MODEL=embeddinggemma \
OPENBRAIN_MCP_BEARER_TOKEN=dev-token \
OPENBRAIN_HTTP_ADDR=127.0.0.1:18080 \
go run ./cmd/openbrain
# in another shell:
curl -i http://127.0.0.1:18080/healthz
```

Output:

```text
HTTP/1.1 200 OK
Date: Sat, 14 Mar 2026 20:19:52 GMT
Content-Length: 2
Content-Type: text/plain; charset=utf-8

ok
```

### 3) MCP auth behavior

Commands:

```bash
curl -i http://127.0.0.1:18081/mcp
curl -i http://127.0.0.1:18081/mcp -H 'Authorization: Bearer dev-token'
```

Output:

```text
--- no token
HTTP/1.1 401 Unauthorized
Date: Sat, 14 Mar 2026 20:20:01 GMT
Content-Length: 12
Content-Type: text/plain; charset=utf-8

unauthorized
--- bearer token
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sat, 14 Mar 2026 20:20:01 GMT
Content-Length: 100

{"tools":["search_thoughts","recent_thoughts","get_thought","stats"],"transport":"streamable-http"}
```

## Open WebUI MCP notes

- set `WEBUI_SECRET_KEY`
- add an external tool of type `MCP (Streamable HTTP)`
- point it at `https://<host>/mcp`
- use bearer auth with the MCP token (`OPENBRAIN_MCP_BEARER_TOKEN`)
