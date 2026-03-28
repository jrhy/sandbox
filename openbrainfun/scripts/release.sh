#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
release_dir="$repo_root/release"

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    printf 'required command not found in PATH: %s\n' "$name" >&2
    exit 1
  fi
}

copy_release_file() {
  local rel="$1"
  local src="$repo_root/$rel"
  local dst="$release_dir/$rel"

  if [[ ! -f "$src" ]]; then
    printf 'required release artifact missing: %s\n' "$src" >&2
    exit 1
  fi

  mkdir -p "$(dirname "$dst")"
  cp "$src" "$dst"
}

copy_release_tree() {
  local rel="$1"
  local src="$repo_root/$rel"
  local dst="$release_dir/$rel"

  if [[ ! -d "$src" ]]; then
    printf 'required release artifact directory missing: %s\n' "$src" >&2
    exit 1
  fi

  mkdir -p "$(dirname "$dst")"
  cp -R "$src" "$dst"
}

write_manifest() {
  local commit_sha
  local tree_state
  local goos
  local goarch

  commit_sha="$(
    cd "$repo_root"
    git rev-parse HEAD
  )"
  tree_state="clean"
  if [[ -n "$(
    cd "$repo_root"
    git status --short --untracked-files=no
  )" ]]; then
    tree_state="dirty"
  fi
  goos="$(
    cd "$repo_root"
    go env GOOS
  )"
  goarch="$(
    cd "$repo_root"
    go env GOARCH
  )"

  {
    printf 'Commit: %s\n' "$commit_sha"
    printf 'Tree: %s\n' "$tree_state"
    printf 'Built: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'GOOS: %s\n' "$goos"
    printf 'GOARCH: %s\n' "$goarch"
  } > "$release_dir/RELEASE_MANIFEST.txt"
}

main() {
  require_command go
  require_command git

  rm -rf "$release_dir"
  mkdir -p "$release_dir"

  (
    cd "$repo_root"
    go build -o "$release_dir/openbrain" ./cmd/openbrain
  )

  copy_release_file ".env.example"
  copy_release_file "scripts/startup-macos.sh"
  copy_release_file "scripts/init-env.sh"
  copy_release_file "docs/operations.md"
  copy_release_tree "migrations"
  write_manifest

  printf 'release bundle ready at %s\n' "$release_dir"
}

main "$@"
