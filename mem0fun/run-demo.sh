#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
log_dir="$script_dir/logs"
mkdir -p "$log_dir"

ts=$(date +%Y%m%d-%H%M%S)
log_file="$log_dir/demo-$ts.log"

docker compose -f "$script_dir/docker-compose.yml" run --name mem0fun-app app python3 /workspace/demo.py 2>&1 | tee "$log_file"
