#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
. "$repo_root/scripts/container-runtime.sh"

export GOWORK="${GOWORK:-off}"
export GOCACHE="${GOCACHE:-$repo_root/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$repo_root/.cache/gomod}"
mkdir -p "$GOCACHE" "$GOMODCACHE"

ollama_url="${OPENBRAIN_OLLAMA_URL:-http://127.0.0.1:11434}"
embed_model="${OPENBRAIN_EMBED_MODEL:-all-minilm:22m}"
metadata_model="${OPENBRAIN_METADATA_MODEL:-qwen3:0.6b}"

pull_ollama_model() {
  local model="$1"
  if command -v ollama >/dev/null 2>&1; then
    ollama pull "$model"
    return
  fi
  container_compose exec -T ollama ollama pull "$model"
}

until curl -fsS "$ollama_url/api/version" >/dev/null; do
  sleep 1
done

pull_ollama_model "$embed_model"
pull_ollama_model "$metadata_model"

OPENBRAIN_OLLAMA_URL="$ollama_url" \
OPENBRAIN_EMBED_MODEL="$embed_model" \
OPENBRAIN_EMBED_BACKEND=ollama \
go test ./internal/embed -run TestOllamaEmbedderSatisfiesContractWhenEnabled -v

OPENBRAIN_OLLAMA_URL="$ollama_url" \
OPENBRAIN_EMBED_MODEL="$embed_model" \
OPENBRAIN_EMBED_BACKEND=ollama \
OPENBRAIN_METADATA_BACKEND=fake \
go test ./internal/e2e -run TestEndToEndCRUDAndMCPIsolation -v

OPENBRAIN_OLLAMA_URL="$ollama_url" \
OPENBRAIN_METADATA_MODEL="$metadata_model" \
OPENBRAIN_METADATA_BACKEND=ollama \
OPENBRAIN_EMBED_BACKEND=fake \
go test ./internal/e2e -run TestMetadataExtractionSmokeWhenEnabled -v
