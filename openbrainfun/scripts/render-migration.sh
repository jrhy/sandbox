#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
project_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
embed_dimensions=${1:-${OPENBRAIN_EMBED_DIMENSIONS:-}}

if [ -z "$embed_dimensions" ]; then
	echo "usage: $0 <embed-dimensions>" >&2
	exit 1
fi

mkdir -p "$project_root/.cache"
tmp_dir=$(mktemp -d "$project_root/.cache/render-migration.XXXXXX")
tmp_go="$tmp_dir/main.go"
trap 'rm -rf "$tmp_dir"' EXIT

cat > "$tmp_go" <<'GOCODE'
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jrhy/sandbox/openbrainfun/internal/migrations"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: render <template-path> <embed-dimensions>")
		os.Exit(1)
	}

	templateBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	embedDimensions, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	rendered, err := migrations.RenderSchema(string(templateBytes), embedDimensions)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Print(rendered)
}
GOCODE

mkdir -p "$project_root/.cache/go-build"
GOWORK=off GOCACHE="$project_root/.cache/go-build" \
	go run "$tmp_go" "$project_root/migrations/0001_initial.sql.tmpl" "$embed_dimensions" \
	> "$project_root/migrations/0001_initial.sql"
