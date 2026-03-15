#!/usr/bin/env bash

container_compose() {
  if command -v podman-compose >/dev/null 2>&1; then
    podman-compose "$@"
    return
  fi
  if command -v podman >/dev/null 2>&1; then
    podman compose "$@"
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    docker compose "$@"
    return
  fi
  echo "neither podman-compose, podman, nor docker is available in PATH" >&2
  return 127
}

setup_go_env() {
  local repo_root="$1"
  export GOWORK=off
  export GOCACHE="${GOCACHE:-$repo_root/.cache/go-build}"
  export GOMODCACHE="${GOMODCACHE:-$repo_root/.cache/gomod}"
  mkdir -p "$GOCACHE" "$GOMODCACHE"
}
