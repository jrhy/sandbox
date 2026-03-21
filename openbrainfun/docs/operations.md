# Local operations

## Persistent data

`compose.yaml` uses explicit bind mounts so persistent state is visible on disk:

- `./var/postgres` — PostgreSQL data directory
- `./var/ollama` — Ollama model cache

Both paths are gitignored and safe to remove only if you intend to destroy local
state.

## Admin CLI

The `openbrain` command now has explicit admin subcommands instead of assuming
the server should start immediately.

Examples:

```bash
go run ./cmd/openbrain
go run ./cmd/openbrain start
OPENBRAIN_DATABASE_URL=postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable \
  go run ./cmd/openbrain user update demo --password demo-password
OPENBRAIN_DATABASE_URL=postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable \
  go run ./cmd/openbrain token create demo --label laptop
```

MCP token plaintext is shown only when a token is created or rotated.

## Local backup and restore

Always take a PostgreSQL backup before upgrading to a release that may change the
schema or trigger large-scale reprocessing. Startup now performs schema
migrations automatically before the app begins serving traffic, so a fresh dump
is the recommended break-glass rollback point if an upgrade fails partway
through.

### PostgreSQL backup

```bash
docker compose exec -T postgres pg_dump -U openbrain -d openbrain > /tmp/openbrainfun.sql
```

### PostgreSQL restore

```bash
cat /tmp/openbrainfun.sql | docker compose exec -T postgres psql -U openbrain -d openbrain
```

### Break-glass restore flow

1. stop the app
2. restore the most recent PostgreSQL dump
3. if you also need to roll back pulled Ollama models, restore `./var/ollama`
4. restart the app on the known-good build

### Ollama cache backup

The Ollama cache is already stored in `./var/ollama`. To preserve models, back
up that directory directly.

## Changing models and re-embedding thoughts

The application keeps embedding and metadata models configurable:

- `OPENBRAIN_EMBED_MODEL` defaults to `all-minilm:22m`
- `OPENBRAIN_METADATA_MODEL` defaults to `qwen3:0.6b`

After changing `OPENBRAIN_EMBED_MODEL`, `OPENBRAIN_METADATA_MODEL`, or the
metadata extraction logic shipped by the app, startup reconciliation marks stale
rows as pending and the worker refreshes only the parts that are out of date.

Operational flow:

1. stop the app
2. take a PostgreSQL backup
3. update the model environment variable and/or deploy the new build
4. pull any newly required Ollama models with `ollama pull <model>`
5. restart the app
6. allow the worker to reprocess stale embeddings and metadata in the background

If a migration or startup reconciliation goes wrong, use the break-glass restore
flow above to roll back to the backup you took before upgrade.
