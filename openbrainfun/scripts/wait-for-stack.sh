#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
. "$repo_root/scripts/container-runtime.sh"

database_url="${OPENBRAIN_DATABASE_URL:-postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable}"
ollama_url="${OPENBRAIN_OLLAMA_URL:-http://127.0.0.1:11434}"
web_health_url="${OPENBRAIN_WEB_HEALTH_URL:-}"

until container_compose exec -T postgres pg_isready -U openbrain -d openbrain >/dev/null 2>&1; do
  sleep 1
done

until curl -fsS "$ollama_url/api/version" >/dev/null; do
  sleep 1
done

if [[ -n "$web_health_url" ]]; then
  until curl -fsS "$web_health_url" >/dev/null; do
    sleep 1
  done
fi

echo "stack ready: $database_url and $ollama_url"
