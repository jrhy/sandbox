#!/bin/bash

# GitHub PR Monitor for SwiftBar
# Uses `gh` CLI for authentication
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

# --- Debug logging ---
DEBUG_LOG="/tmp/swiftbar-debug.log"

log_debug() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "$DEBUG_LOG"
}

# Run a command and log everything
run_cmd() {
  local output
  local exit_code
  log_debug "RUN: $*"
  output=$("$@" 2>&1)
  exit_code=$?
  if [[ $exit_code -ne 0 ]]; then
    log_debug "FAILED (exit $exit_code)"
    log_debug "OUTPUT: $output"
  else
    log_debug "OK (exit 0), output length: ${#output}"
  fi
  echo "$output"
  return $exit_code
}

# --- Configuration ---
CACHE_FILE="/tmp/swiftbar-pr-cache.txt"
IDLE_THRESHOLD=60

log_debug "========== Script started =========="

# Check idle time - skip API calls if user is idle
idle_seconds=$(ioreg -c IOHIDSystem 2>/dev/null | awk '/HIDIdleTime/ {print int($NF/1000000000); exit}')
log_debug "Idle seconds: $idle_seconds"

if [[ -n "$idle_seconds" && "$idle_seconds" -gt "$IDLE_THRESHOLD" && -f "$CACHE_FILE" ]]; then
  log_debug "User idle ($idle_seconds s), using cached output"
  # Read cached output, add sleeping emoji to first line
  first_line=$(head -1 "$CACHE_FILE")
  echo "${first_line/ |/ 😴 |}"
  tail -n +2 "$CACHE_FILE"
  exit 0
fi

log_debug "PATH=$PATH"
log_debug "HOME=$HOME"
log_debug "USER=$USER"
log_debug "PWD=$PWD"
log_debug "gh location: $(which gh 2>&1)"
log_debug "jq location: $(which jq 2>&1)"

GITHUB_USERNAME=$(run_cmd gh api user --jq '.login')
log_debug "GITHUB_USERNAME=$GITHUB_USERNAME"
if [[ -z "$GITHUB_USERNAME" ]]; then
  log_debug "Failed to get GitHub username - aborting"
  echo "Error | color=red"
  echo "---"
  echo "Could not get GitHub username"
  exit 0
fi

# --- Fetch PRs where you're involved ---
QUERY_OPEN="is:pr is:open involves:${GITHUB_USERNAME} sort:updated-desc"
QUERY_MERGED="is:pr is:merged involves:${GITHUB_USERNAME} sort:updated-desc"

prs_json=$(run_cmd gh api "search/issues?q=$(echo "$QUERY_OPEN" | jq -sRr @uri)&per_page=8")
merged_json=$(run_cmd gh api "search/issues?q=$(echo "$QUERY_MERGED" | jq -sRr @uri)&per_page=5")
log_debug "prs_json length: ${#prs_json}"

# --- Parse PRs and collect their check statuses ---
overall_status="green"

declare -a my_pr_lines=()
declare -a other_pr_lines=()
declare -a statuses=()

pr_data=$(echo "$prs_json" | jq -r '.items[:8] | .[] | [.number, .pull_request.url, .title, .html_url, .repository_url, .user.login] | @tsv')
log_debug "pr_data: $pr_data"

while IFS= read -r line; do
  [[ -z "$line" ]] && continue
  log_debug "Processing PR line: $line"

  IFS=$'\t' read -r pr_number pr_url title html_url repo_url author <<< "$line"

  # Extract owner/repo from repository_url
  repo_path=$(echo "$repo_url" | sed 's|https://api.github.com/repos/||')

  # Get the PR details to find the head SHA
  pr_detail=$(run_cmd gh api "repos/${repo_path}/pulls/${pr_number}")
  head_sha=$(echo "$pr_detail" | jq -r '.head.sha')

  # Get check runs for this commit (per_page=100 to avoid missing checks beyond default 30)
  checks=$(run_cmd gh api "repos/${repo_path}/commits/${head_sha}/check-runs?per_page=100")

  # Determine status: any failure = red, any in_progress/queued = yellow, else green
  has_failure=$(echo "$checks" | jq '[.check_runs[].conclusion] | any(. == "failure" or . == "cancelled")')
  has_pending=$(echo "$checks" | jq '[.check_runs[].status] | any(. == "in_progress" or . == "queued")')

  if [[ "$has_failure" == "true" ]]; then
    pr_status="red"
    status_icon="xmark.circle.fill"
  elif [[ "$has_pending" == "true" ]]; then
    pr_status="yellow"
    status_icon="clock.circle.fill"
  else
    pr_status="green"
    status_icon="checkmark.circle.fill"
  fi

  # Truncate title if too long
  short_title="${title:0:50}"
  [[ ${#title} -gt 50 ]] && short_title="${short_title}..."

  # Sanitize title: remove | to prevent SwiftBar parameter injection
  safe_title="${short_title//|/-}"

  pr_line="$pr_status"$'\t'"$status_icon"$'\t'"$safe_title"$'\t'"$html_url"$'\t'"$repo_path"$'\t'"$pr_number"

  # Separate my PRs from others; only my PRs affect overall status
  if [[ "$author" == "$GITHUB_USERNAME" ]]; then
    my_pr_lines+=("$pr_line")
    # Only count toward overall status if not a POC/WIP PR
    if [[ ! "$title" =~ ^(POC|WIP)(:|[[:space:]]) ]]; then
      statuses+=("$pr_status")
    else
      log_debug "Skipping status for POC/WIP PR: $title"
    fi
  else
    other_pr_lines+=("$pr_line")
    log_debug "Skipping status for other's PR (author=$author): $title"
  fi

done <<< "$pr_data"

log_debug "After loop: ${#my_pr_lines[@]} my PRs, ${#other_pr_lines[@]} other PRs"

# --- Calculate overall status (only from my PRs) ---
# Yellow (pending) takes priority - things are in flux
# Red (failed) only shows when nothing is pending
for s in "${statuses[@]}"; do
  if [[ "$s" == "yellow" ]]; then
    overall_status="yellow"
    break
  elif [[ "$s" == "red" && "$overall_status" != "yellow" ]]; then
    overall_status="red"
  fi
done

# --- Menu bar icon ---
log_debug "overall_status=$overall_status, pr_lines count=${#pr_lines[@]}"

# Helper to echo, log, and cache
cache_output=""
out() {
  echo "$@"
  log_debug "OUT: $*"
  cache_output+="$*"$'\n'
}

case "$overall_status" in
  red)    out "🔴 | color=red" ;;
  yellow) out "🟡 | color=yellow" ;;
  green)  out "🟢 | color=green" ;;
esac

out "---"

# --- My PRs ---
if [[ ${#my_pr_lines[@]} -eq 0 ]]; then
  out "No open PRs by me"
else
  for pr_line in "${my_pr_lines[@]}"; do
    IFS=$'\t' read -r status icon title url repo pr_num <<< "$pr_line"
    out "$title | image=SF:$icon color=$status href=$url"
    out "-- ${repo}#${pr_num} | color=gray size=11"
  done
fi

# --- Other PRs I'm involved in ---
if [[ ${#other_pr_lines[@]} -gt 0 ]]; then
  out "---"
  out "Involved In"
  for pr_line in "${other_pr_lines[@]}"; do
    IFS=$'\t' read -r status icon title url repo pr_num <<< "$pr_line"
    out "$title | image=SF:$icon color=$status href=$url"
    out "-- ${repo}#${pr_num} | color=gray size=11"
  done
fi

# --- Merged PRs ---
merged_data=$(echo "$merged_json" | jq -r '.items[:5] | .[] | [.number, .title, .html_url, .user.login] | @tsv')
declare -a my_merged_lines=()
declare -a other_merged_lines=()

while IFS=$'\t' read -r pr_number title html_url author; do
  [[ -z "$pr_number" ]] && continue

  # Truncate and sanitize
  short_title="${title:0:40}"
  [[ ${#title} -gt 40 ]] && short_title="${short_title}..."
  safe_title="${short_title//|/-}"

  if [[ "$author" == "$GITHUB_USERNAME" ]]; then
    my_merged_lines+=("$safe_title"$'\t'"$html_url")
  else
    other_merged_lines+=("$author"$'\t'"$safe_title"$'\t'"$html_url")
  fi
done <<< "$merged_data"

if [[ ${#my_merged_lines[@]} -gt 0 ]]; then
  out "---"
  out "Recently Merged"
  for pr_line in "${my_merged_lines[@]}"; do
    IFS=$'\t' read -r title url <<< "$pr_line"
    out "$title | image=SF:checkmark.circle.fill color=purple href=$url"
  done
fi

if [[ ${#other_merged_lines[@]} -gt 0 ]]; then
  out "---"
  out "Involved In (Merged)"
  for pr_line in "${other_merged_lines[@]}"; do
    IFS=$'\t' read -r author title url <<< "$pr_line"
    out "$author: $title | image=SF:checkmark.circle.fill color=purple href=$url"
  done
fi

out "---"
out "Open PRs | href=https://github.com/pulls?q=is%3Aopen+involves%3A${GITHUB_USERNAME}"
out "All PRs (inc. closed) | href=https://github.com/pulls?q=involves%3A${GITHUB_USERNAME}"
out "Refresh | refresh=true"
# Save output to cache for idle mode
echo -n "$cache_output" > "$CACHE_FILE"
log_debug "========== Script finished =========="
