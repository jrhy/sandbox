#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

env_file="${OPENBRAIN_ENV_FILE:-$repo_root/.env}"
load_env_file() {
  if [[ -f "$env_file" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$env_file"
    set +a
  fi
}

load_env_file

container_bin="${OPENBRAIN_CONTAINER_BIN:-/usr/local/bin/container}"
postgres_container_name="${OPENBRAIN_POSTGRES_CONTAINER_NAME:-openbrain-postgres}"
postgres_image="${OPENBRAIN_POSTGRES_IMAGE:-pgvector/pgvector:pg16}"
postgres_host_port="${OPENBRAIN_POSTGRES_HOST_PORT:-5432}"
postgres_db="${OPENBRAIN_POSTGRES_DB:-openbrain}"
postgres_user="${OPENBRAIN_POSTGRES_USER:-openbrain}"
postgres_password="${OPENBRAIN_POSTGRES_PASSWORD:-openbrain}"
postgres_data_dir="${OPENBRAIN_POSTGRES_DATA_DIR:-}"
postgres_volume_name="${OPENBRAIN_POSTGRES_VOLUME_NAME:-openbrain-postgres-data}"
postgres_pgdata="${OPENBRAIN_POSTGRES_PGDATA:-/var/lib/postgresql/data/pgdata}"
postgres_ready_timeout_seconds="${OPENBRAIN_POSTGRES_READY_TIMEOUT_SECONDS:-30}"

export OPENBRAIN_WEB_ADDR="${OPENBRAIN_WEB_ADDR:-127.0.0.1:18080}"
export OPENBRAIN_MCP_ADDR="${OPENBRAIN_MCP_ADDR:-127.0.0.1:18081}"
export OPENBRAIN_DATABASE_URL="${OPENBRAIN_DATABASE_URL:-postgres://${postgres_user}:${postgres_password}@127.0.0.1:${postgres_host_port}/${postgres_db}?sslmode=disable}"
export OPENBRAIN_OLLAMA_URL="${OPENBRAIN_OLLAMA_URL:-http://127.0.0.1:11434}"
export OPENBRAIN_EMBED_MODEL="${OPENBRAIN_EMBED_MODEL:-all-minilm:22m}"
export OPENBRAIN_METADATA_MODEL="${OPENBRAIN_METADATA_MODEL:-qwen3:0.6b}"
export OPENBRAIN_SESSION_TTL="${OPENBRAIN_SESSION_TTL:-24h}"
export OPENBRAIN_COOKIE_SECURE="${OPENBRAIN_COOKIE_SECURE:-false}"
export OPENBRAIN_LOG_LEVEL="${OPENBRAIN_LOG_LEVEL:-info}"

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    printf 'required command not found in PATH: %s\n' "$name" >&2
    exit 1
  fi
}

ensure_csrf_key() {
  if [[ -z "${OPENBRAIN_CSRF_KEY:-}" ]]; then
    printf 'OPENBRAIN_CSRF_KEY is required; set it in %s or export it before running this script\n' "$env_file" >&2
    printf 'hint: scripts/init-env.sh creates a .env with a generated CSRF key\n' >&2
    exit 1
  fi
  export OPENBRAIN_CSRF_KEY
}

ensure_container_system() {
  "$container_bin" system start >/dev/null
}

postgres_mount_arg() {
  if [[ -n "$postgres_data_dir" ]]; then
    mkdir -p "$postgres_data_dir"
    printf '%s:/var/lib/postgresql/data' "$postgres_data_dir"
    return
  fi
  printf '%s:/var/lib/postgresql/data' "$postgres_volume_name"
}

postgres_inspect_json() {
  "$container_bin" inspect "$postgres_container_name" 2>/dev/null || true
}

postgres_container_exists() {
  postgres_inspect_json \
    | python3 -c 'import json,sys
try:
    payload=json.load(sys.stdin)
except Exception:
    sys.exit(1)
sys.exit(0 if isinstance(payload, list) and len(payload) > 0 else 1)
'
}

postgres_container_status() {
  if ! postgres_container_exists; then
    return 1
  fi

  postgres_inspect_json \
    | python3 -c 'import json,sys; payload=json.load(sys.stdin); print((payload[0] or {}).get("status",""))'
}

print_postgres_logs() {
  local logs
  if ! logs="$("$container_bin" logs "$postgres_container_name" 2>&1)"; then
    printf 'postgres container %s logs unavailable:\n%s\n' "$postgres_container_name" "$logs" >&2
    return
  fi
  if [[ -n "$logs" ]]; then
    printf 'postgres container %s logs:\n%s\n' "$postgres_container_name" "$logs" >&2
  fi
}

start_postgres_container() {
  if "$container_bin" exec "$postgres_container_name" pg_isready -U "$postgres_user" -d "$postgres_db" >/dev/null 2>&1; then
    return
  fi

  if postgres_container_exists; then
    if ! "$container_bin" start "$postgres_container_name" >/dev/null 2>&1; then
      printf 'failed to start existing postgres container %s\n' "$postgres_container_name" >&2
      print_postgres_logs
      exit 1
    fi
    return
  fi

  "$container_bin" run \
    --detach \
    --name "$postgres_container_name" \
    --env "POSTGRES_DB=$postgres_db" \
    --env "POSTGRES_USER=$postgres_user" \
    --env "POSTGRES_PASSWORD=$postgres_password" \
    --env "PGDATA=$postgres_pgdata" \
    --publish "127.0.0.1:${postgres_host_port}:5432" \
    --volume "$(postgres_mount_arg)" \
    "$postgres_image" >/dev/null
}

wait_for_postgres() {
  local elapsed_seconds=0
  local container_status=""

  until "$container_bin" exec "$postgres_container_name" pg_isready -U "$postgres_user" -d "$postgres_db" >/dev/null 2>&1; do
    container_status="$(postgres_container_status 2>/dev/null || true)"
    if [[ "$container_status" == "stopped" || "$container_status" == "exited" || "$container_status" == "dead" ]]; then
      printf 'postgres container %s entered terminal state: %s\n' "$postgres_container_name" "$container_status" >&2
      print_postgres_logs
      printf 'if this container was created with an incompatible bind mount, remove it with:\n  %s rm %s\nthen rerun this script so it can recreate the container.\n' "$container_bin" "$postgres_container_name" >&2
      exit 1
    fi
    if (( elapsed_seconds >= postgres_ready_timeout_seconds )); then
      printf 'timed out waiting %ss for postgres container %s to become ready\n' "$postgres_ready_timeout_seconds" "$postgres_container_name" >&2
      print_postgres_logs
      exit 1
    fi
    sleep 1
    ((elapsed_seconds += 1))
  done
}

ensure_schema() {
  local schema_exists
  schema_exists="$("$container_bin" exec "$postgres_container_name" psql -tAc "select 1 from pg_tables where schemaname = 'public' and tablename = 'users'" -U "$postgres_user" -d "$postgres_db" | tr -d '[:space:]')"
  if [[ "$schema_exists" == "1" ]]; then
    return
  fi

  "$container_bin" exec --interactive "$postgres_container_name" psql -v ON_ERROR_STOP=1 -U "$postgres_user" -d "$postgres_db" < migrations/0001_initial.sql
}

ensure_ollama() {
  if ! curl -fsS "${OPENBRAIN_OLLAMA_URL}/api/version" >/dev/null; then
    printf 'ollama is not reachable at %s; start or port-forward it before running this script\n' "$OPENBRAIN_OLLAMA_URL" >&2
    exit 1
  fi
}

warn_if_cookie_secure_requires_https() {
  if [[ "${OPENBRAIN_COOKIE_SECURE}" == "true" ]]; then
    printf 'warning: OPENBRAIN_COOKIE_SECURE=true means browser login cookies are only sent over HTTPS; direct HTTP access to %s will bounce back to /login unless you put OpenBrainFun behind an HTTPS reverse proxy.\n' "$OPENBRAIN_WEB_ADDR" >&2
  fi
}

main() {
  if [[ ! -x "$container_bin" ]]; then
    printf 'required macOS container runtime not found or not executable: %s\n' "$container_bin" >&2
    exit 1
  fi

  require_command curl
  require_command python3
  require_command openbrain

  ensure_csrf_key
  ensure_container_system
  start_postgres_container
  wait_for_postgres
  ensure_schema
  ensure_ollama
  warn_if_cookie_secure_requires_https

  if [[ -n "$postgres_data_dir" ]]; then
    printf 'starting openbrain with postgres container %s using bind mount %s and ollama at %s\n' "$postgres_container_name" "$postgres_data_dir" "$OPENBRAIN_OLLAMA_URL"
  else
    printf 'starting openbrain with postgres container %s using volume %s and ollama at %s\n' "$postgres_container_name" "$postgres_volume_name" "$OPENBRAIN_OLLAMA_URL"
  fi
  exec openbrain start
}

main "$@"
