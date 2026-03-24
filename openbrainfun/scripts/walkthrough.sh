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
  stop_app
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

announce_phase() {
  printf '\n==> %s\n' "$1"
}

parse_json_field_from_http_response() {
  local field="$1"

  python3 -c '
import json, sys
field = sys.argv[1]
payload = sys.stdin.read().split("\r\n\r\n")[-1].split("\n\n")[-1]
print(json.loads(payload)[field])
' "$field"
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

create_thought() {
  local title="$1"
  local request_json="$2"
  local display_command="$3"

  last_capture_output="$(curl -sS -i -b "$cookie_jar" -H 'Content-Type: application/json' -H "X-CSRF-Token: $csrf_token" \
    -X POST "http://${OPENBRAIN_WEB_ADDR}/api/thoughts" \
    --data "$request_json")"
  record_step "$title" "$display_command" "$last_capture_output"
}

stop_app() {
  local job_pid
  for job_pid in $(jobs -pr); do
    kill "$job_pid" >/dev/null 2>&1 || true
    wait "$job_pid" 2>/dev/null || true
  done
  if [[ -n "${app_pid:-}" ]] && kill -0 "$app_pid" >/dev/null 2>&1; then
    kill "$app_pid" >/dev/null 2>&1 || true
    wait "$app_pid" 2>/dev/null || true
  fi
  unset app_pid
}

database_host_port() {
  python3 - <<'PY' "$OPENBRAIN_DATABASE_URL"
import sys
from urllib.parse import urlparse

parsed = urlparse(sys.argv[1])
host = parsed.hostname or ""
port = parsed.port or 5432
print(f"{host} {port}")
PY
}

check_for_conflicting_local_postgres_runtime() {
  local db_host db_port listeners pid cmdline
  if ! command -v lsof >/dev/null 2>&1; then
    return
  fi

  read -r db_host db_port <<<"$(database_host_port)"
  if [[ "$db_host" != "127.0.0.1" && "$db_host" != "localhost" ]]; then
    return
  fi

  listeners="$(lsof -nP -iTCP:"$db_port" -sTCP:LISTEN 2>/dev/null || true)"
  pid="$(printf '%s\n' "$listeners" | awk -v port="$db_port" '$0 ~ ("127\\.0\\.0\\.1:" port " \\(LISTEN\\)") && $1 == "container" { print $2; exit }')"
  if [[ -z "$pid" ]]; then
    return
  fi

  cmdline="$(ps -p "$pid" -o command= 2>/dev/null || true)"
  if [[ "$cmdline" != *"container-runtime-linux"* || "$cmdline" != *"openbrain-postgres"* ]]; then
    return
  fi

  cat >&2 <<EOF
conflicting local postgres runtime detected on 127.0.0.1:$db_port

The walkthrough uses Podman compose, but port $db_port is already owned by the macOS container runtime container "openbrain-postgres":
  $cmdline

Stop/remove it first:
  container stop openbrain-postgres
  container rm openbrain-postgres

Then recreate the Podman postgres data and rerun:
  podman compose down
  rm -rf var/postgres
  ./scripts/walkthrough.sh
EOF
  exit 1
}

check_for_conflicting_local_postgres_runtime

announce_phase "Starting local stack"
container_compose up -d postgres ollama
bash ./scripts/wait-for-stack.sh

announce_phase "Pulling Ollama models"
container_compose exec -T ollama ollama pull "$OPENBRAIN_EMBED_MODEL"
container_compose exec -T ollama ollama pull "$OPENBRAIN_METADATA_MODEL"

announce_phase "Resetting demo schema"
container_compose exec -T postgres psql -v ON_ERROR_STOP=1 -U openbrain -d openbrain <<'SQL'
drop table if exists thoughts cascade;
drop table if exists mcp_tokens cascade;
drop table if exists web_sessions cascade;
drop table if exists users cascade;
SQL
container_compose exec -T postgres psql -v ON_ERROR_STOP=1 -U openbrain -d openbrain < migrations/0001_initial.sql

announce_phase "Provisioning demo user"
run_capture "Create or update the demo user" "go run ./cmd/openbrain user update '${OPENBRAIN_DEMO_USERNAME}' --password '${OPENBRAIN_DEMO_PASSWORD}' --token-label '${OPENBRAIN_DEMO_TOKEN_LABEL}'"

demo_mcp_token="$(printf '%s\n' "$last_capture_output" | awk -F= '/^token=/{print $2}')"
if [[ -z "$demo_mcp_token" ]]; then
  printf 'failed to parse demo MCP token from openbrain user update output\n%s\n' "$last_capture_output" >&2
  exit 1
fi

announce_phase "Starting OpenBrain server"
go run ./cmd/openbrain start >"$app_log" 2>&1 &
app_pid=$!
export OPENBRAIN_WEB_HEALTH_URL="http://${OPENBRAIN_WEB_ADDR}/healthz"
bash ./scripts/wait-for-stack.sh

announce_phase "Logging in"
run_capture "Log in and receive a CSRF token" "curl -sS -i -c '$cookie_jar' -H 'Content-Type: application/json' -X POST 'http://${OPENBRAIN_WEB_ADDR}/api/session' --data '{\"username\":\"${OPENBRAIN_DEMO_USERNAME}\",\"password\":\"${OPENBRAIN_DEMO_PASSWORD}\"}'"

csrf_token="$(
  curl -sS -c "$cookie_jar" -H 'Content-Type: application/json' -X POST "http://${OPENBRAIN_WEB_ADDR}/api/session" \
    --data "{\"username\":\"${OPENBRAIN_DEMO_USERNAME}\",\"password\":\"${OPENBRAIN_DEMO_PASSWORD}\"}" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["csrf_token"])'
)"

announce_phase "Creating demo thoughts"
create_thought \
  "Create a baseline auth thought" \
  '{"content":"Remember MCP auth and local sessions for Open WebUI","exposure_scope":"remote_ok","user_tags":["mcp","sessions"]}' \
  "curl -sS -i -b '$cookie_jar' -H 'Content-Type: application/json' -H 'X-CSRF-Token: <csrf-token>' -X POST 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts' --data '{\"content\":\"Remember MCP auth and local sessions for Open WebUI\",\"exposure_scope\":\"remote_ok\",\"user_tags\":[\"mcp\",\"sessions\"]}'"
auth_baseline_id="$(printf '%s' "$last_capture_output" | parse_json_field_from_http_response id)"

create_thought \
  "Create an unrelated gardening thought" \
  '{"content":"Prune the balcony tomato plants and water the seedlings on Tuesday","exposure_scope":"remote_ok","user_tags":["garden","plants"]}' \
  "curl -sS -i -b '$cookie_jar' -H 'Content-Type: application/json' -H 'X-CSRF-Token: <csrf-token>' -X POST 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts' --data '{\"content\":\"Prune the balcony tomato plants and water the seedlings on Tuesday\",\"exposure_scope\":\"remote_ok\",\"user_tags\":[\"garden\",\"plants\"]}'"
garden_id="$(printf '%s' "$last_capture_output" | parse_json_field_from_http_response id)"

create_thought \
  "Create an unrelated shopping thought" \
  '{"content":"Buy coffee beans, oats, and oranges after work","exposure_scope":"remote_ok","user_tags":["shopping","groceries"]}' \
  "curl -sS -i -b '$cookie_jar' -H 'Content-Type: application/json' -H 'X-CSRF-Token: <csrf-token>' -X POST 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts' --data '{\"content\":\"Buy coffee beans, oats, and oranges after work\",\"exposure_scope\":\"remote_ok\",\"user_tags\":[\"shopping\",\"groceries\"]}'"
shopping_id="$(printf '%s' "$last_capture_output" | parse_json_field_from_http_response id)"

create_thought \
  "Create the anchor thought for related-thought search" \
  '{"content":"Local MCP bearer tokens should stay tied to one user session","exposure_scope":"remote_ok","user_tags":["mcp","auth"]}' \
  "curl -sS -i -b '$cookie_jar' -H 'Content-Type: application/json' -H 'X-CSRF-Token: <csrf-token>' -X POST 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts' --data '{\"content\":\"Local MCP bearer tokens should stay tied to one user session\",\"exposure_scope\":\"remote_ok\",\"user_tags\":[\"mcp\",\"auth\"]}'"
thought_id="$(printf '%s' "$last_capture_output" | parse_json_field_from_http_response id)"

thought_ready_timeout_seconds="${OPENBRAIN_WALKTHROUGH_READY_TIMEOUT_SECONDS:-60}"
announce_phase "Waiting for background processing"
wait_for_ready_response "$auth_baseline_id" "$thought_ready_timeout_seconds" >/dev/null
wait_for_ready_response "$garden_id" "$thought_ready_timeout_seconds" >/dev/null
wait_for_ready_response "$shopping_id" "$thought_ready_timeout_seconds" >/dev/null
get_response="$(wait_for_ready_response "$thought_id" "$thought_ready_timeout_seconds")"
record_step "Retrieve the anchor thought after background processing" "curl -sS -i -b '$cookie_jar' 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts/${thought_id}'" "$get_response"

announce_phase "Querying related thoughts"
related_response="$(curl -sS -i -b "$cookie_jar" "http://${OPENBRAIN_WEB_ADDR}/api/thoughts/${thought_id}/related?limit=3")"
record_step "Find related thoughts through the JSON API" "curl -sS -i -b '$cookie_jar' 'http://${OPENBRAIN_WEB_ADDR}/api/thoughts/${thought_id}/related?limit=3'" "$related_response"

mcp_response="$(curl -sS -i -H 'Content-Type: application/json' -H "Authorization: Bearer ${demo_mcp_token}" \
  -X POST "http://${OPENBRAIN_MCP_ADDR}/mcp" \
  --data "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"related_thoughts\",\"arguments\":{\"id\":\"${thought_id}\",\"limit\":3}}}")"
record_step "Find related thoughts through MCP" "curl -sS -i -H 'Content-Type: application/json' -H 'Authorization: Bearer ${demo_mcp_token}' -X POST 'http://${OPENBRAIN_MCP_ADDR}/mcp' --data '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"related_thoughts\",\"arguments\":{\"id\":\"${thought_id}\",\"limit\":3}}}'" "$mcp_response"

stop_app

expected_step_count=8
if (( ${#step_titles[@]} < expected_step_count )); then
  printf 'expected at least %d walkthrough steps, got %d\n' "$expected_step_count" "${#step_titles[@]}" >&2
  printf 'recorded step titles:\n' >&2
  printf ' - %s\n' "${step_titles[@]}" >&2
  exit 1
fi

export STEP_TITLES="$(IFS=$'\x1f'; printf '%s' "${step_titles[*]}")"
export STEP_COMMANDS="$(IFS=$'\x1f'; printf '%s' "${step_commands[*]}")"
export STEP_RESPONSES="$(IFS=$'\x1f'; printf '%s' "${step_responses[*]}")"

announce_phase "Writing walkthrough doc"
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
printf 'Full transcript: docs/walkthrough.demo.md\n'
