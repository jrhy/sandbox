# OpenBrainFun Local Walkthrough

*2026-03-14T20:19:30Z by Showboat 0.6.1*
<!-- showboat-id: 7db8707b-237c-44d8-816c-ecc54689877d -->

This demo shows MCP token provisioning and local endpoint verification for OpenBrainFun.

```bash
uvx showboat --help | sed -n "1,16p"
```

```output
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

```bash
export OPENBRAIN_MCP_BEARER_TOKEN=dev-token; echo OPENBRAIN_MCP_BEARER_TOKEN=$OPENBRAIN_MCP_BEARER_TOKEN
```

```output
OPENBRAIN_MCP_BEARER_TOKEN=dev-token
```

```bash
OPENBRAIN_DATABASE_URL=postgres://x OPENBRAIN_OLLAMA_URL=http://127.0.0.1:11434 OPENBRAIN_EMBED_MODEL=embeddinggemma OPENBRAIN_MCP_BEARER_TOKEN=dev-token OPENBRAIN_HTTP_ADDR=127.0.0.1:18080 go run ./cmd/openbrain >/tmp/openbrainfun-demo.log 2>&1 & pid=$!; sleep 1; curl --max-time 5 -sS -i http://127.0.0.1:18080/healthz; if kill -0 $pid 2>/dev/null; then kill $pid; fi; wait $pid 2>/dev/null || true
```

```output
HTTP/1.1 200 OK
Date: Sat, 14 Mar 2026 20:19:52 GMT
Content-Length: 2
Content-Type: text/plain; charset=utf-8

ok
```

```bash
OPENBRAIN_DATABASE_URL=postgres://x OPENBRAIN_OLLAMA_URL=http://127.0.0.1:11434 OPENBRAIN_EMBED_MODEL=embeddinggemma OPENBRAIN_MCP_BEARER_TOKEN=dev-token OPENBRAIN_HTTP_ADDR=127.0.0.1:18081 go run ./cmd/openbrain >/tmp/openbrainfun-demo.log 2>&1 & pid=$!; sleep 1; echo "--- no token"; curl --max-time 5 -sS -i http://127.0.0.1:18081/mcp; echo; echo "--- bearer token"; curl --max-time 5 -sS -i http://127.0.0.1:18081/mcp -H "Authorization: Bearer dev-token"; if kill -0 $pid 2>/dev/null; then kill $pid; fi; wait $pid 2>/dev/null || true
```

```output
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
