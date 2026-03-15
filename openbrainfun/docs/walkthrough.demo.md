## Walkthrough

This section is generated from `./scripts/walkthrough.sh` using real interactions. For readability, long `curl` commands are wrapped and JSON bodies are pretty-printed.

### Persistent data

Persistent data lives in:
- `./var/postgres`
- `./var/ollama`

### Models used in this walkthrough

- Embedding model: `all-minilm:22m`
- Metadata model: `qwen3:0.6b`

_Generated at 2026-03-15T20:38:45Z._

### Create or update the demo user

```bash
go run ./cmd/openbrain user update 'demo' --password 'demo-password' --token-label 'default'
```

```text
updated user username=demo
created default token label=default
token=0iVIrWugi86uafwQqB7OmpwAjhEmPaHfR2jTKyPrzFk
note: this token will not be shown again; use `openbrain token create demo --label default` to rotate it
```

### Log in and receive a CSRF token

```bash
curl \
  -sS \
  -i \
  -c /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.uow4zqwdM8 \
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
Set-Cookie: openbrain_session=1fUpZF7qPmGzHFvy-g0s1bFIoITRIK0P0WhRDGFoVrQ; Path=/; Expires=Mon, 16 Mar 2026 20:38:32 GMT; HttpOnly; SameSite=Lax
Date: Sun, 15 Mar 2026 20:38:32 GMT
Content-Length: 82

{
  "csrf_token": "f37c315b5ea2c005ce25518e89bc714580efdaa115a967f44a5af9eae1695a2b"
}
```

### Create a thought

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.uow4zqwdM8 \
  -H 'Content-Type: application/json' \
  -H 'X-CSRF-Token: <csrf-token>' \
  -X POST \
  http://127.0.0.1:18080/api/thoughts \
  --data '{
  "content": "Remember MCP auth and local sessions",
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
Date: Sun, 15 Mar 2026 20:38:32 GMT
Content-Length: 326

{
  "id": "2d7f2618-277a-4800-aec2-5f5ff26c0a0a",
  "content": "Remember MCP auth and local sessions",
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
  "created_at": "2026-03-15T20:38:32Z",
  "updated_at": "2026-03-15T20:38:32Z"
}
```

### Retrieve the thought after background processing

```bash
curl \
  -sS \
  -i \
  -b /var/folders/qt/pxvds2pn1qnd2blwq356wtxh0000gq/T/tmp.uow4zqwdM8 \
  http://127.0.0.1:18080/api/thoughts/2d7f2618-277a-4800-aec2-5f5ff26c0a0a
```

```text
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 15 Mar 2026 20:38:45 GMT
Content-Length: 738

{
  "id": "2d7f2618-277a-4800-aec2-5f5ff26c0a0a",
  "content": "Remember MCP auth and local sessions",
  "exposure_scope": "remote_ok",
  "user_tags": [
    "mcp",
    "sessions"
  ],
  "metadata": {
    "Summary": "Concise summary: MCP authentication and local session management are key for secure access. Topics: Authentication protocols, session management strategies, security best practices.",
    "Topics": [
      "MCP auth",
      "local sessions",
      "authentication protocols",
      "session management strategies",
      "security best practices"
    ],
    "Entities": [
      "MCP",
      "local sessions",
      "authentication protocols",
      "session management strategies",
      "security best practices"
    ]
  },
  "embedding_model": "all-minilm:22m",
  "ingest_status": "ready",
  "created_at": "2026-03-15T20:38:32Z",
  "updated_at": "2026-03-15T20:38:45Z"
}
```

### Query MCP

```bash
curl \
  -sS \
  -i \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer 0iVIrWugi86uafwQqB7OmpwAjhEmPaHfR2jTKyPrzFk' \
  -X POST \
  http://127.0.0.1:18081/mcp \
  --data '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "search_thoughts",
    "arguments": {
      "query": "MCP auth"
    }
  }
}'
```

```text
HTTP/1.1 200 OK
Cache-Control: no-cache, no-transform
Content-Type: application/json
Date: Sun, 15 Mar 2026 20:38:45 GMT
Content-Length: 504

{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"thoughts\":[{\"content\":\"Remember MCP auth and local sessions\",\"exposure_scope\":\"remote_ok\",\"id\":\"2d7f2618-277a-4800-aec2-5f5ff26c0a0a\",\"ingest_status\":\"ready\",\"user_tags\":[\"mcp\",\"sessions\"]}]}"
      }
    ],
    "structuredContent": {
      "thoughts": [
        {
          "content": "Remember MCP auth and local sessions",
          "exposure_scope": "remote_ok",
          "id": "2d7f2618-277a-4800-aec2-5f5ff26c0a0a",
          "ingest_status": "ready",
          "user_tags": [
            "mcp",
            "sessions"
          ]
        }
      ]
    }
  }
}
```
