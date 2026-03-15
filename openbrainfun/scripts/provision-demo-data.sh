#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
. "$repo_root/scripts/container-runtime.sh"
setup_go_env "$repo_root"

username="${OPENBRAIN_DEMO_USERNAME:-demo}"
password="${OPENBRAIN_DEMO_PASSWORD:-demo-password}"
token="${OPENBRAIN_DEMO_MCP_TOKEN:-demo-mcp-token}"
label="${OPENBRAIN_DEMO_MCP_LABEL:-demo}"

tmp_dir="$(mktemp -d "$repo_root/.cache/provision-demo-data.XXXXXX")"
tmp_go="$tmp_dir/main.go"
trap 'rm -rf "$tmp_dir"' EXIT

cat >"$tmp_go" <<'EOF'
package main

import (
	"fmt"
	"os"

	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

func main() {
	passwordHash, err := auth.HashPassword(os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Printf("password_hash=%s\n", passwordHash)
	fmt.Printf("token_hash=%s\n", auth.HashToken(os.Args[2]))
}
EOF

hash_output="$(go run "$tmp_go" "$password" "$token")"
password_hash="$(printf '%s\n' "$hash_output" | awk -F= '/^password_hash=/{print $2}')"
token_hash="$(printf '%s\n' "$hash_output" | awk -F= '/^token_hash=/{print $2}')"

container_compose exec -T postgres psql -U openbrain -d openbrain \
  -v username="$username" \
  -v password_hash="$password_hash" \
  -v token_hash="$token_hash" \
  -v label="$label" <<'SQL'
insert into users (username, password_hash)
values (:'username', :'password_hash')
on conflict (username) do update
set password_hash = excluded.password_hash,
    updated_at = now()
returning id \gset

delete from mcp_tokens
where user_id = :'id' and label = :'label';

insert into mcp_tokens (user_id, token_hash, label)
values (:'id', :'token_hash', :'label');
SQL

printf 'provisioned username=%s token=%s label=%s\n' "$username" "$token" "$label"
