#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <coverprofile>" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

export GOWORK="${GOWORK:-off}"
export GOCACHE="${GOCACHE:-$repo_root/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$repo_root/.cache/gomod}"
mkdir -p "$GOCACHE" "$GOMODCACHE"

profile="$1"
if [[ ! -f "$profile" ]]; then
  echo "coverage profile not found: $profile" >&2
  exit 1
fi

min_total="${OPENBRAIN_COVERAGE_MIN_TOTAL:-54.0}"

# These are ratcheted from the current baseline so coverage cannot silently
# regress while the remaining implementation work continues.
core_packages=(
  "./internal/app:100.0"
  "./internal/e2e:60.0"
  "./internal/mcpserver:55.0"
  "./internal/worker:70.0"
)

extract_total() {
  local cover_file="$1"
  go tool cover -func="$cover_file" | awk '/^total:/ { sub(/%/, "", $3); print $3 }'
}

check_floor() {
  local label="$1"
  local actual="$2"
  local floor="$3"
  python3 - "$label" "$actual" "$floor" <<'PY'
import sys
label, actual, floor = sys.argv[1], float(sys.argv[2]), float(sys.argv[3])
if actual + 1e-9 < floor:
    print(f"{label} coverage {actual:.1f}% is below floor {floor:.1f}%", file=sys.stderr)
    raise SystemExit(1)
print(f"{label} coverage {actual:.1f}% >= floor {floor:.1f}%")
PY
}

check_floor "total" "$(extract_total "$profile")" "$min_total"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

for entry in "${core_packages[@]}"; do
  pkg="${entry%%:*}"
  floor="${entry##*:}"
  pkg_name="${pkg//\//_}"
  pkg_profile="$tmpdir/${pkg_name}.out"
  go test "$pkg" -coverprofile="$pkg_profile" >/tmp/openbrainfun-coverage-${pkg_name}.log
  check_floor "$pkg" "$(extract_total "$pkg_profile")" "$floor"
done
