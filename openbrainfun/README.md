# OpenBrainFun

OpenBrainFun is a local-first, multi-user “open brain” system built in Go.

It is directly inspired by Nate B Jones’ Open Brain guide:

- <https://promptkit.natebjones.com/20260224_uq1_guide_main>

The goal is similar: keep thoughts in a store that can be searched and reused
through MCP by external AI clients. The main infrastructural differences are:

- it is self-hosted and local-first rather than built around hosted services
- it replaces Supabase with Postgres + pgvector
- it replaces Slack capture with a built-in web UI and JSON API
- it replaces remote embedding dependencies with Ollama running locally

Separately, OpenBrainFun also chooses DB-backed browser sessions, per-user
thought and MCP isolation from day one, and a repo that treats the README
walkthrough, local operations docs, and verification scripts as first-class
project artifacts.

In practice, OpenBrainFun provides:

- a password-protected browser UI for capturing and editing thoughts
- a separate MCP endpoint on its own port, authenticated by bearer token
- scored semantic search and related-thought lookups through the JSON API and MCP
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
below. The rendered walkthrough wraps long `curl` commands and pretty-prints
JSON bodies so the checked-in transcript stays readable while remaining
grounded in real requests and responses. It seeds several intentionally
different thoughts plus one new auth-related thought, then demonstrates
similar-thought lookup through both `GET /api/thoughts/{id}/related` and the
MCP `related_thoughts` tool. That makes the walkthrough prove that the
embedding store is doing real retrieval work, not just storing vectors.

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

See [docs/operations.md](docs/operations.md) for backup/restore notes,
automatic startup migrations, and the model-reconciliation flow used when
embedding or metadata configuration changes.

## TODO

- Expose the semantic-search threshold knob in the web UI and JSON API.
  MCP search already accepts a `threshold` argument, but the browser flow still
  uses the default threshold implicitly.

<!-- walkthrough:start -->

## Walkthrough

This section is generated from `./scripts/walkthrough.sh` using real interactions. For readability, long `curl` commands are wrapped and JSON bodies are pretty-printed.

### Persistent data

Persistent data lives in:
- `./var/postgres`
- `./var/ollama`

### Models used in this walkthrough

- Embedding model: `all-minilm:22m`
- Metadata model: `qwen3:0.6b`

_Generated at 2026-03-15T22:41:43Z._

### Create or update the demo user

```bash
go run ./cmd/openbrain user update 'demo' --password 'demo-password' --token-label 'default'
```

```text
updated user username=demo
created default token label=default
token=tjBWpAcQlXzEVaZQKU7TUF3fTlRo-zDhvWnD12f3xhw
note: this token will not be shown again; use `openbrain token create demo --label default` to rotate it
```

### Log in and receive a CSRF token

```bash
curl \
  -sS \
  -i \
  -c /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.TEwFk2AwQI \
  -H 'Content-Type: application/json' \
  -X POST \
  http://127.0.0.1:18080/api/session \
  --data '{
  "username": "demo",
  "password": "demo-password"
}'
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Set-Cookie: openbrain_session=dRY_RJ_GFvdPxS9U-_ArpXqzad3MDHUbvP7KcOdtkdA; Path=/; Expires=Mon, 16 Mar 2026 22:41:02 GMT; HttpOnly; SameSite=Lax
Date: Sun, 15 Mar 2026 22:41:02 GMT
Content-Length: 82

{
  "csrf_token": "1955b2910bdfcc4eb8dd58a456e03b34352a1368fc430d305a39a171d7520dc3"
}
```

### Create a baseline auth thought

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.TEwFk2AwQI \
  -H 'Content-Type: application/json' \
  -H 'X-CSRF-Token: <csrf-token>' \
  -X POST \
  http://127.0.0.1:18080/api/thoughts \
  --data '{
  "content": "Remember MCP auth and local sessions for Open WebUI",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "mcp",
    "sessions"
  ]
}'
```

```text
HTTP/1.1 201 Created
Content-Type: application/json
Date: Sun, 15 Mar 2026 22:41:02 GMT
Content-Length: 341

{
  "id": "94e6b2c3-1d8d-4420-b0de-f37eb3585bc0",
  "content": "Remember MCP auth and local sessions for Open WebUI",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "mcp",
    "sessions"
  ],
  "metadata": {
    "Summary": "No summary available.",
    "Topics": [],
    "Entities": []
  },
  "ingest_status": "pending",
  "created_at": "2026-03-15T22:41:02Z",
  "updated_at": "2026-03-15T22:41:02Z"
}
```

### Create an unrelated gardening thought

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.TEwFk2AwQI \
  -H 'Content-Type: application/json' \
  -H 'X-CSRF-Token: <csrf-token>' \
  -X POST \
  http://127.0.0.1:18080/api/thoughts \
  --data '{
  "content": "Prune the balcony tomato plants and water the seedlings on Tuesday",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "garden",
    "plants"
  ]
}'
```

```text
HTTP/1.1 201 Created
Content-Type: application/json
Date: Sun, 15 Mar 2026 22:41:02 GMT
Content-Length: 357

{
  "id": "5531659c-12ad-4b10-bdb3-926600d95504",
  "content": "Prune the balcony tomato plants and water the seedlings on Tuesday",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "garden",
    "plants"
  ],
  "metadata": {
    "Summary": "No summary available.",
    "Topics": [],
    "Entities": []
  },
  "ingest_status": "pending",
  "created_at": "2026-03-15T22:41:02Z",
  "updated_at": "2026-03-15T22:41:02Z"
}
```

### Create an unrelated shopping thought

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.TEwFk2AwQI \
  -H 'Content-Type: application/json' \
  -H 'X-CSRF-Token: <csrf-token>' \
  -X POST \
  http://127.0.0.1:18080/api/thoughts \
  --data '{
  "content": "Buy coffee beans, oats, and oranges after work",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "shopping",
    "groceries"
  ]
}'
```

```text
HTTP/1.1 201 Created
Content-Type: application/json
Date: Sun, 15 Mar 2026 22:41:02 GMT
Content-Length: 342

{
  "id": "b4bb2e65-ade8-4bf6-b0ba-2a5d5ed08a1f",
  "content": "Buy coffee beans, oats, and oranges after work",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "shopping",
    "groceries"
  ],
  "metadata": {
    "Summary": "No summary available.",
    "Topics": [],
    "Entities": []
  },
  "ingest_status": "pending",
  "created_at": "2026-03-15T22:41:02Z",
  "updated_at": "2026-03-15T22:41:02Z"
}
```

### Create the anchor thought for related-thought search

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.TEwFk2AwQI \
  -H 'Content-Type: application/json' \
  -H 'X-CSRF-Token: <csrf-token>' \
  -X POST \
  http://127.0.0.1:18080/api/thoughts \
  --data '{
  "content": "Local MCP bearer tokens should stay tied to one user session",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "mcp",
    "auth"
  ]
}'
```

```text
HTTP/1.1 201 Created
Content-Type: application/json
Date: Sun, 15 Mar 2026 22:41:02 GMT
Content-Length: 346

{
  "id": "a2883658-db81-40e7-a779-ca597b3c12de",
  "content": "Local MCP bearer tokens should stay tied to one user session",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "mcp",
    "auth"
  ],
  "metadata": {
    "Summary": "No summary available.",
    "Topics": [],
    "Entities": []
  },
  "ingest_status": "pending",
  "created_at": "2026-03-15T22:41:02Z",
  "updated_at": "2026-03-15T22:41:02Z"
}
```

### Retrieve the anchor thought after background processing

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.TEwFk2AwQI \
  http://127.0.0.1:18080/api/thoughts/a2883658-db81-40e7-a779-ca597b3c12de
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 15 Mar 2026 22:41:43 GMT
Content-Length: 543

{
  "id": "a2883658-db81-40e7-a779-ca597b3c12de",
  "content": "Local MCP bearer tokens should stay tied to one user session",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "mcp",
    "auth"
  ],
  "metadata": {
    "Summary": "Local MCP bearer tokens should be tied to a single user session to ensure security and consistency.",
    "Topics": [
      "user session",
      "bearer tokens",
      "local systems"
    ],
    "Entities": [
      "Local MCP bearer tokens",
      "user session"
    ]
  },
  "embedding_model": "all-minilm:22m",
  "ingest_status": "ready",
  "created_at": "2026-03-15T22:41:02Z",
  "updated_at": "2026-03-15T22:41:13Z"
}
```

### Find related thoughts through the JSON API

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.TEwFk2AwQI \
  'http://127.0.0.1:18080/api/thoughts/a2883658-db81-40e7-a779-ca597b3c12de/related?limit=3'
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 15 Mar 2026 22:41:43 GMT
Content-Length: 1626

{
  "thoughts": [
    {
      "id": "94e6b2c3-1d8d-4420-b0de-f37eb3585bc0",
      "content": "Remember MCP auth and local sessions for Open WebUI",
      "exposure_scope": "remote_ok",
      "user_tags": [
        "mcp",
        "sessions"
      ],
      "metadata": {
        "Summary": "Keep track of MCP authentication and local sessions in Open WebUI for secure and efficient user sessions.",
        "Topics": [
          "authentication",
          "local sessions",
          "Open WebUI"
        ],
        "Entities": [
          "MCP",
          "Open WebUI",
          "session management",
          "authentication"
        ]
      },
      "similarity": 0.5801512253194822,
      "embedding_model": "all-minilm:22m",
      "ingest_status": "ready",
      "created_at": "2026-03-15T22:41:02Z",
      "updated_at": "2026-03-15T22:41:43Z"
    },
    {
      "id": "b4bb2e65-ade8-4bf6-b0ba-2a5d5ed08a1f",
      "content": "Buy coffee beans, oats, and oranges after work",
      "exposure_scope": "remote_ok",
      "user_tags": [
        "shopping",
        "groceries"
      ],
      "metadata": {
        "Summary": "Buy coffee beans, oats, and oranges after work",
        "Topics": [
          "coffee beans",
          "oats",
          "oranges"
        ],
        "Entities": [
          "coffee beans",
          "oats",
          "oranges"
        ]
      },
      "similarity": 0.0652156216291715,
      "embedding_model": "all-minilm:22m",
      "ingest_status": "ready",
      "created_at": "2026-03-15T22:41:02Z",
      "updated_at": "2026-03-15T22:41:24Z"
    },
    {
      "id": "5531659c-12ad-4b10-bdb3-926600d95504",
      "content": "Prune the balcony tomato plants and water the seedlings on Tuesday",
      "exposure_scope": "remote_ok",
      "user_tags": [
        "garden",
        "plants"
      ],
      "metadata": {
        "Summary": "Prune tomato plants and water seedlings on Tuesday.",
        "Topics": [
          "pruning tomato plants",
          "watering seedlings"
        ],
        "Entities": [
          "tomato plants",
          "seedlings"
        ]
      },
      "similarity": -0.05749998494982411,
      "embedding_model": "all-minilm:22m",
      "ingest_status": "ready",
      "created_at": "2026-03-15T22:41:02Z",
      "updated_at": "2026-03-15T22:41:35Z"
    }
  ]
}
```

### Find related thoughts through MCP

```bash
curl \
  -sS \
  -i \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer tjBWpAcQlXzEVaZQKU7TUF3fTlRo-zDhvWnD12f3xhw' \
  -X POST \
  http://127.0.0.1:18081/mcp \
  --data '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "related_thoughts",
    "arguments": {
      "id": "a2883658-db81-40e7-a779-ca597b3c12de",
      "limit": 3
    }
  }
}'
```

```text
HTTP/1.1 200 OK
Cache-Control: no-cache, no-transform
Content-Type: application/json
Date: Sun, 15 Mar 2026 22:41:43 GMT
Content-Length: 1590

{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"thoughts\":[{\"content\":\"Remember MCP auth and local sessions for Open WebUI\",\"exposure_scope\":\"remote_ok\",\"id\":\"94e6b2c3-1d8d-4420-b0de-f37eb3585bc0\",\"ingest_status\":\"ready\",\"similarity\":0.5801512253194822,\"user_tags\":[\"mcp\",\"sessions\"]},{\"content\":\"Buy coffee beans, oats, and oranges after work\",\"exposure_scope\":\"remote_ok\",\"id\":\"b4bb2e65-ade8-4bf6-b0ba-2a5d5ed08a1f\",\"ingest_status\":\"ready\",\"similarity\":0.0652156216291715,\"user_tags\":[\"shopping\",\"groceries\"]},{\"content\":\"Prune the balcony tomato plants and water the seedlings on Tuesday\",\"exposure_scope\":\"remote_ok\",\"id\":\"5531659c-12ad-4b10-bdb3-926600d95504\",\"ingest_status\":\"ready\",\"similarity\":-0.05749998494982411,\"user_tags\":[\"garden\",\"plants\"]}]}"
      }
    ],
    "structuredContent": {
      "thoughts": [
        {
          "content": "Remember MCP auth and local sessions for Open WebUI",
          "exposure_scope": "remote_ok",
          "id": "94e6b2c3-1d8d-4420-b0de-f37eb3585bc0",
          "ingest_status": "ready",
          "similarity": 0.5801512253194822,
          "user_tags": [
            "mcp",
            "sessions"
          ]
        },
        {
          "content": "Buy coffee beans, oats, and oranges after work",
          "exposure_scope": "remote_ok",
          "id": "b4bb2e65-ade8-4bf6-b0ba-2a5d5ed08a1f",
          "ingest_status": "ready",
          "similarity": 0.0652156216291715,
          "user_tags": [
            "shopping",
            "groceries"
          ]
        },
        {
          "content": "Prune the balcony tomato plants and water the seedlings on Tuesday",
          "exposure_scope": "remote_ok",
          "id": "5531659c-12ad-4b10-bdb3-926600d95504",
          "ingest_status": "ready",
          "similarity": -0.05749998494982411,
          "user_tags": [
            "garden",
            "plants"
          ]
        }
      ]
    }
  }
}
```

<!-- walkthrough:end -->
