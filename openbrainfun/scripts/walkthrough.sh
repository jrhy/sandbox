#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
. "$repo_root/scripts/container-runtime.sh"
setup_go_env "$repo_root"

export OPENBRAIN_WEB_ADDR="${OPENBRAIN_WEB_ADDR:-127.0.0.1:18080}"
export OPENBRAIN_MCP_ADDR="${OPENBRAIN_MCP_ADDR:-127.0.0.1:18081}"
export OPENBRAIN_DATABASE_URL="${OPENBRAIN_DATABASE_URL:-postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable}"
export OPENBRAIN_OLLAMA_URL="${OPENBRAIN_OLLAMA_URL:-http://127.0.0.1:11434}"
export OPENBRAIN_EMBED_MODEL="${OPENBRAIN_EMBED_MODEL:-all-minilm:22m}"
export OPENBRAIN_METADATA_MODEL="${OPENBRAIN_METADATA_MODEL:-qwen3:0.6b}"
export OPENBRAIN_SESSION_TTL="${OPENBRAIN_SESSION_TTL:-24h}"
export OPENBRAIN_COOKIE_SECURE="${OPENBRAIN_COOKIE_SECURE:-false}"
export OPENBRAIN_CSRF_KEY="${OPENBRAIN_CSRF_KEY:-walkthrough-csrf-key}"
export OPENBRAIN_LOG_LEVEL="${OPENBRAIN_LOG_LEVEL:-info}"
export OPENBRAIN_DEMO_USERNAME="${OPENBRAIN_DEMO_USERNAME:-demo}"
export OPENBRAIN_DEMO_PASSWORD="${OPENBRAIN_DEMO_PASSWORD:-demo-password}"
export OPENBRAIN_DEMO_TOKEN_LABEL="${OPENBRAIN_DEMO_TOKEN_LABEL:-default}"

mkdir -p docs var/postgres var/ollama

cookie_jar="$(mktemp)"
body_file="$(mktemp)"
headers_file="$(mktemp)"
app_log="$(mktemp)"
transcript_json="$(mktemp)"
render_tmp_dir="$(mktemp -d "$repo_root/.cache/walkthrough-render.XXXXXX")"
render_go="$render_tmp_dir/main.go"
cleanup() {
  rm -f "$cookie_jar" "$body_file" "$headers_file" "$transcript_json"
  rm -rf "$render_tmp_dir"
  if [[ -n "${app_pid:-}" ]] && kill -0 "$app_pid" >/dev/null 2>&1; then
    kill "$app_pid"
    wait "$app_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

step_titles=()
step_commands=()
step_responses=()
last_capture_output=""

record_step() {
  step_titles+=("$1")
  step_commands+=("$2")
  step_responses+=("$3")
}

run_capture() {
  local title="$1"
  local command="$2"
  local output
  output="$(bash -c "$command")"
  last_capture_output="$output"
  record_step "$title" "$command" "$output"
}

wait_for_ready_response() {
  local thought_id="$1"
  local timeout_seconds="${2:-60}"
  local elapsed_seconds=0
  local response=""
  local ingest_status=""

  while (( elapsed_seconds < timeout_seconds )); do
    response="$(curl -sS -i -b "$cookie_jar" "http://${OPENBRAIN_WEB_ADDR}/api/thoughts/${thought_id}")"
    ingest_status="$(
      printf '%s' "$response" | python3 -c 'import json,sys; payload=sys.stdin.read().split("\r\n\r\n")[-1].split("\n\n")[-1]; print(json.loads(payload)["ingest_status"])'
    )"
    if [[ "$ingest_status" == "ready" ]]; then
      printf '%s' "$response"
      return 0
    fi
    sleep 1
    ((elapsed_seconds += 1))
  done

  printf 'timed out waiting for thought %s to become ready\nlast response:\n%s\n' "$thought_id" "$response" >&2
  return 1
}

container_compose up -d postgres ollama
bash ./scripts/wait-for-stack.sh

container_compose exec -T ollama ollama pull "$OPENBRAIN_EMBED_MODEL"
container_compose exec -T ollama ollama pull "$OPENBRAIN_METADATA_MODEL"

container_compose exec -T postgres psql -v ON_ERROR_STOP=1 -U openbrain -d openbrain <<'SQL'
drop table if exists thoughts cascade;
drop table if exists mcp_tokens cascade;
drop table if exists web_sessions cascade;
drop table if exists users cascade;
SQL

container_compose exec -T postgres psql -v ON_ERROR_STOP=1 -U openbrain -d openbrain < migrations/0001_initial.sql

run_capture "Create or update the demo user" "go run ./cmd/openbrain user update '${OPENBRAIN_DEMO_USERNAME}' --password '${OPENBRAIN_DEMO_PASSWORD}' --token-label '${OPENBRAIN_DEMO_TOKEN_LABEL}'"

demo_mcp_token="$(printf '%s\n' "$last_capture_output" | awk -F= '/^token=/{print $2}')"
if [[ -z "$demo_mcp_token" ]]; then
  printf 'failed to parse demo MCP token from openbrain user update output\n%s\n' "$last_capture_output" >&2
  exit 1
fi

go run ./cmd/openbrain start >"$app_log" 2>&1 &
app_pid=$!
export OPENBRAIN_WEB_HEALTH_URL="http://${OPENBRAIN_WEB_ADDR}/healthz"
bash ./scripts/wait-for-stack.sh

run_capture "Log in and receive a CSRF token" "curl -sS -i -c '$cookie_jar' -H 'Content-Type: application/json' -X POST 'http://${OPENBRAIN_WEB_ADDR}/api/session' --data '{\"username\":\"${OPENBRAIN_DEMO_USERNAME}\",\"password\":\"${OPENBRAIN_DEMO_PASSWORD}\"}'"

csrf_token="$(
  curl -sS -c "$cookie_jar" -H 'Content-Type: application/json' -X POST "http://${OPENBRAIN_WEB_ADDR}/api/session" \
    --data "{\"username\":\"${OPENBRAIN_DEMO_USERNAME}\",\"password\":\"${OPENBRAIN_DEMO_PASSWORD}\"}" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["csrf_token"])'
)"

create_response="$(curl -sS -i -b "$cookie_jar" -H 'Content-Type: application/json' -H "X-CSRF-Token: $csrf_token" \
  -X POST "http://${OPENBRAIN_WEB_ADDR}/api/thoughts" \
  --data '{"content":"Remember MCP auth and local sessions","exposure_scope":"remote_ok","user_tags":["mcp","sessions"]}')"
record_step "Create a thought" "curl -sS -i -b '$cookie_jar' -H 'Content-Type: application/json' -H 'X-CSRF-Token: <csrf-token>' -X POST 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts' --data '{\"content\":\"Remember MCP auth and local sessions\",\"exposure_scope\":\"remote_ok\",\"user_tags\":[\"mcp\",\"sessions\"]}'" "$create_response"

thought_id="$(
  printf '%s' "$create_response" | python3 -c 'import json,sys; payload=sys.stdin.read().split("\r\n\r\n")[-1].split("\n\n")[-1]; print(json.loads(payload)["id"])'
)"

thought_ready_timeout_seconds="${OPENBRAIN_WALKTHROUGH_READY_TIMEOUT_SECONDS:-60}"
get_response="$(wait_for_ready_response "$thought_id" "$thought_ready_timeout_seconds")"
record_step "Retrieve the thought after background processing" "curl -sS -i -b '$cookie_jar' 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts/${thought_id}'" "$get_response"

mcp_response="$(curl -sS -i -H 'Content-Type: application/json' -H "Authorization: Bearer ${demo_mcp_token}" \
  -X POST "http://${OPENBRAIN_MCP_ADDR}/mcp" \
  --data '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_thoughts","arguments":{"query":"MCP auth"}}}')"
record_step "Query MCP" "curl -sS -i -H 'Content-Type: application/json' -H 'Authorization: Bearer ${demo_mcp_token}' -X POST 'http://${OPENBRAIN_MCP_ADDR}/mcp' --data '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"search_thoughts\",\"arguments\":{\"query\":\"MCP auth\"}}}'" "$mcp_response"

export STEP_TITLES="$(IFS=$'\x1f'; printf '%s' "${step_titles[*]}")"
export STEP_COMMANDS="$(IFS=$'\x1f'; printf '%s' "${step_commands[*]}")"
export STEP_RESPONSES="$(IFS=$'\x1f'; printf '%s' "${step_responses[*]}")"

python3 - "$transcript_json" "$OPENBRAIN_EMBED_MODEL" "$OPENBRAIN_METADATA_MODEL" <<'PY'
import json, sys, os
path, embed_model, metadata_model = sys.argv[1], sys.argv[2], sys.argv[3]
titles = os.environ["STEP_TITLES"].split("\x1f")
commands = os.environ["STEP_COMMANDS"].split("\x1f")
responses = os.environ["STEP_RESPONSES"].split("\x1f")
payload = {
    "generated_at": __import__("datetime").datetime.utcnow().replace(microsecond=0).isoformat() + "Z",
    "storage_paths": ["./var/postgres", "./var/ollama"],
    "embed_model": embed_model,
    "metadata_model": metadata_model,
    "steps": [{"title": t, "command": c, "response": r} for t, c, r in zip(titles, commands, responses)],
}
with open(path, "w", encoding="utf-8") as fh:
    json.dump(payload, fh)
PY

cat >"$render_go" <<'EOF'
package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/jrhy/sandbox/openbrainfun/internal/readmegen"
)

type transcriptJSON struct {
	GeneratedAt   string `json:"generated_at"`
	StoragePaths  []string `json:"storage_paths"`
	EmbedModel    string `json:"embed_model"`
	MetadataModel string `json:"metadata_model"`
	Steps         []struct {
		Title    string `json:"title"`
		Command  string `json:"command"`
		Response string `json:"response"`
	} `json:"steps"`
}

func main() {
	payloadPath := os.Args[1]
	readmePath := os.Args[2]
	walkthroughPath := os.Args[3]

	raw, err := os.ReadFile(payloadPath)
	if err != nil {
		panic(err)
	}
	var payload transcriptJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		panic(err)
	}
	generatedAt, err := time.Parse(time.RFC3339, payload.GeneratedAt)
	if err != nil {
		panic(err)
	}
	steps := make([]readmegen.Step, 0, len(payload.Steps))
	for _, step := range payload.Steps {
		steps = append(steps, readmegen.Step{
			Title: step.Title,
			Command: step.Command,
			Response: step.Response,
		})
	}
	walkthrough, err := readmegen.RenderWalkthrough(readmegen.Transcript{
		GeneratedAt: generatedAt,
		StoragePaths: payload.StoragePaths,
		EmbedModel: payload.EmbedModel,
		MetadataModel: payload.MetadataModel,
		Steps: steps,
	})
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(walkthroughPath, []byte(walkthrough), 0o644); err != nil {
		panic(err)
	}
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		panic(err)
	}
	updatedReadme, err := readmegen.UpdateREADME(string(readme), walkthrough)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(readmePath, []byte(updatedReadme), 0o644); err != nil {
		panic(err)
	}
}
EOF

go run "$render_go" "$transcript_json" README.md docs/walkthrough.demo.md
