#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

export GOWORK="${GOWORK:-off}"
export GOCACHE="${GOCACHE:-$repo_root/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$repo_root/.cache/gomod}"
mkdir -p "$GOCACHE" "$GOMODCACHE"

fd_cmd="fd"
if ! command -v "$fd_cmd" >/dev/null 2>&1; then
  fd_cmd="fdfind"
fi
if ! command -v "$fd_cmd" >/dev/null 2>&1; then
  echo "fd (or fdfind) is required" >&2
  exit 1
fi

go_files=()
while IFS= read -r file; do
  go_files+=("$file")
done < <("$fd_cmd" -e go . cmd internal)
if [[ "${#go_files[@]}" -eq 0 ]]; then
  echo "no Go files found under cmd/ or internal/" >&2
  exit 1
fi

unformatted="$(gofmt -l "${go_files[@]}")"
if [[ -n "$unformatted" ]]; then
  echo "gofmt mismatch:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./...

cover_profile="${COVER_PROFILE:-/tmp/openbrainfun-cover.out}"
go test ./... -coverprofile="$cover_profile" -v

"$repo_root/scripts/check-coverage.sh" "$cover_profile"
git diff --check -- .
