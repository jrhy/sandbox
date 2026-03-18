#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

env_template="${OPENBRAIN_ENV_TEMPLATE:-$repo_root/.env.example}"
env_file="${OPENBRAIN_ENV_FILE:-$repo_root/.env}"
force=0

usage() {
  cat <<'EOF'
Usage: scripts/init-env.sh [--force]

Create a local .env from .env.example with generated secrets.

Options:
  --force    overwrite an existing .env file
EOF
}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    printf 'required command not found in PATH: %s\n' "$name" >&2
    exit 1
  fi
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --force)
        force=1
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        printf 'unknown argument: %s\n\n' "$1" >&2
        usage >&2
        exit 1
        ;;
    esac
    shift
  done
}

main() {
  parse_args "$@"
  require_command python3

  if [[ ! -f "$env_template" ]]; then
    printf 'env template not found: %s\n' "$env_template" >&2
    exit 1
  fi

  if [[ -e "$env_file" && "$force" -ne 1 ]]; then
    printf '%s already exists; rerun with --force to overwrite it\n' "$env_file" >&2
    exit 1
  fi

  mkdir -p "$(dirname "$env_file")"
  umask 077

  local postgres_password
  local csrf_key
  postgres_password="$(python3 -c 'import secrets; print(secrets.token_urlsafe(24))')"
  csrf_key="$(python3 -c 'import secrets; print(secrets.token_hex(32))')"

  local tmp
  tmp="$(mktemp "${env_file}.tmp.XXXXXX")"
  python3 - "$env_template" "$tmp" "$postgres_password" "$csrf_key" <<'PY'
import pathlib
import sys

import urllib.parse

template_path = pathlib.Path(sys.argv[1])
output_path = pathlib.Path(sys.argv[2])
postgres_password = sys.argv[3]
csrf_key = sys.argv[4]

text = template_path.read_text(encoding="utf-8")
text = text.replace("__GENERATE_POSTGRES_PASSWORD__", postgres_password)
text = text.replace("__GENERATE_CSRF_KEY__", csrf_key)

values = {}
for line in text.splitlines():
    stripped = line.strip()
    if not stripped or stripped.startswith("#") or "=" not in stripped:
        continue
    key, value = stripped.split("=", 1)
    values[key] = value

database_url = "postgres://{user}:{password}@127.0.0.1:{port}/{database}?sslmode=disable".format(
    user=urllib.parse.quote(values["OPENBRAIN_POSTGRES_USER"], safe=""),
    password=urllib.parse.quote(values["OPENBRAIN_POSTGRES_PASSWORD"], safe=""),
    port=values["OPENBRAIN_POSTGRES_HOST_PORT"],
    database=urllib.parse.quote(values["OPENBRAIN_POSTGRES_DB"], safe=""),
)
text = text.replace("__GENERATE_DATABASE_URL__", database_url)

if "__GENERATE_" in text:
    raise SystemExit("template placeholders were not fully replaced")

output_path.write_text(text, encoding="utf-8")
PY

  chmod 600 "$tmp"
  mv "$tmp" "$env_file"

  printf 'wrote %s\n' "$env_file"
  printf 'review it, then provision app users with `openbrain user update ...`\n'
}

main "$@"
