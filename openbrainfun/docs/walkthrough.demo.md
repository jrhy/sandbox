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
