# Local operations

## Persistent data

`compose.yaml` uses explicit bind mounts so persistent state is visible on disk:

- `./var/postgres` — PostgreSQL data directory
- `./var/ollama` — Ollama model cache

Both paths are gitignored and safe to remove only if you intend to destroy local
state.

## Local backup and restore

### PostgreSQL backup

```bash
docker compose exec -T postgres pg_dump -U openbrain -d openbrain > /tmp/openbrainfun.sql
```

### PostgreSQL restore

```bash
cat /tmp/openbrainfun.sql | docker compose exec -T postgres psql -U openbrain -d openbrain
```

### Ollama cache backup

The Ollama cache is already stored in `./var/ollama`. To preserve models, back
up that directory directly.

## Changing models and re-embedding thoughts

The application keeps embedding and metadata models configurable:

- `OPENBRAIN_EMBED_MODEL` defaults to `all-minilm:22m`
- `OPENBRAIN_METADATA_MODEL` defaults to `qwen3:0.6b`

After changing `OPENBRAIN_EMBED_MODEL`, existing thoughts should be re-embedded
so vector dimensions and search behavior stay consistent.

Recommended local flow:

1. stop the app
2. update the model environment variable
3. pull the new model with `ollama pull <model>`
4. reset thought ingest state in Postgres so thoughts return to `pending`
5. restart the app and let the worker reprocess thoughts

Example reset:

```sql
update thoughts
set ingest_status = 'pending',
    ingest_error = '',
    embedding = null,
    embedding_model = null,
    updated_at = now();
```
