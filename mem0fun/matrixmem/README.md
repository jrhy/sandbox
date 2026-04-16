# Matrix Mem0 Kuzu Bot

This directory runs the single-user Matrix bot that stores raw events locally, writes memories into Mem0, and keeps the vector and graph stores on disk.

## What it persists

- `data/archive` for raw Matrix event history
- `data/qdrant` for Qdrant vector data
- `data/kuzu` for the local Kuzu database files
- `logs` for application logs

## Layout

- `qdrant` runs as a Docker service and keeps its storage on `data/qdrant`
- `app` is the Debian-based Python container that runs the Matrix bot loop
- Kuzu is embedded in the app container and stored on the bind-mounted `data/kuzu` path
- raw archive files and bot logs are kept on bind mounts so they survive restarts

## Start

From this directory:

```bash
cp .env.example .env
mkdir -p data/archive data/qdrant data/kuzu logs
docker compose up -d --build
docker compose logs -f app
```

## Matrix Setup

Copy `.env.example` to `.env` and fill in the Matrix values before starting the bot:

- `MATRIX_HOMESERVER_URL` is the full homeserver URL, for example `https://matrix.example`
- `MATRIX_ACCESS_TOKEN` is the access token for the Matrix account that will run the bot
- `MATRIX_ROOM_ID` is the real internal room id, usually starting with `!`, not just a room alias

The token-owning account must already be joined to that room. The bot currently listens to exactly one room, only handles `m.text` events, ignores its own messages, and treats any message containing `?` as a query instead of a memory to store.

To watch the vector store too:

```bash
docker compose logs -f qdrant
```

To verify the compose file parses cleanly:

```bash
docker compose config
```

## Restart

The bot and Qdrant service both use `restart: unless-stopped`, so they come back automatically after a daemon restart or crash.

```bash
docker compose restart app
docker compose restart qdrant
```

To stop the stack without losing state, run:

```bash
docker compose down
```

Bring it back later with:

```bash
docker compose up -d
```

Because the archive, Qdrant data, Kuzu database, and logs all live on bind mounts, those files remain on disk across restarts and `down`/`up` cycles.

## Harness

The local harness drives the bot without Matrix credentials. It supports a fast mock mode for deterministic checks and a real mode that uses the containerized Mem0, Qdrant, Kuzu, and Ollama stack.

Run the deterministic mock scenario on the host:

```bash
PYTHONPATH=../.. python3 -m mem0fun.matrixmem.harness --mode mock
```

Run the real scenario inside the app container after Qdrant is up:

```bash
docker compose up -d qdrant
docker compose run --rm app python3 -m mem0fun.matrixmem.harness \
  --mode real \
  --archive-path /data/archive/harness-events.jsonl \
  --room-id '!harness:local' \
  --user-id '@harness:local'
```

To verify persistence after a restart, bring the stack down and back up, then rerun the harness in query-only mode with the same room and user ids:

```bash
docker compose down
docker compose up -d qdrant
docker compose run --rm app python3 -m mem0fun.matrixmem.harness \
  --mode real \
  --archive-path /data/archive/harness-events.jsonl \
  --room-id '!harness:local' \
  --user-id '@harness:local' \
  --skip-ingest
```

In real mode, expect the exact wording of the answer to vary less than the shape of the result. The useful checks are that the archive file exists, the query returns at least one relevant memory, and the same memory is still retrievable after the restart.

## Current Limits

- The mock harness is covered by automated tests, but the `--mode real` path is still a documented operator workflow rather than an automated pass/fail test
- Live Matrix room verification is still manual; the bot startup and room wiring are ready, but we have not yet added an automated smoke test for a real homeserver
- The bot currently handles one room and one event shape well; edits, deletes, media, and richer event types are future work

## Next Steps

- Add an opt-in integration test that runs the harness in `--mode real` and makes shape-based assertions instead of exact-text assertions
- Add a real Matrix room smoke-test flow once the homeserver and room details are available
- Keep building toward the next-step aspiration in [docs/next-step-agentic-planner.md](/Users/jeffr/sandbox/mem0fun/matrixmem/docs/next-step-agentic-planner.md:1), where retrieval stays grounded in the archive while a planner revisits stored memories to suggest next actions

## Notes

- `OLLAMA_BASE_URL` points at the host's Ollama process through `host.docker.internal`
- `QDRANT_URL` points at the `qdrant` service inside the Compose network
- Copy `.env.example` to `.env` before starting the stack; the `app` service loads that file directly so the Matrix credentials and optional Mem0 defaults are available inside the container
