#!/bin/bash

# Jira MRU (Most Recently Used) for SwiftBar
# Assumes jira CLI is in PATH (set by wrapper script if needed)

# --- Debug logging ---
DEBUG_LOG="/tmp/swiftbar-debug.log"
CACHE_FILE="/tmp/swiftbar-jira-cache.txt"
IDLE_THRESHOLD=60

log_debug() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] [jira] $*" >> "$DEBUG_LOG"
}

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

log_debug "========== Script started =========="

# Check idle time - skip API calls if user is idle
idle_seconds=$(ioreg -c IOHIDSystem 2>/dev/null | awk '/HIDIdleTime/ {print int($NF/1000000000); exit}')
log_debug "Idle seconds: $idle_seconds"

if [[ -n "$idle_seconds" && "$idle_seconds" -gt "$IDLE_THRESHOLD" && -f "$CACHE_FILE" ]]; then
  log_debug "User idle ($idle_seconds s), using cached output"
  first_line=$(head -1 "$CACHE_FILE")
  echo "${first_line/ |/ 😴 |}"
  tail -n +2 "$CACHE_FILE"
  exit 0
fi

# Check if jira CLI is available
if ! command -v jira &> /dev/null; then
  log_debug "jira CLI not found"
  echo "Jira | color=red"
  echo "---"
  echo "jira CLI not found"
  exit 0
fi

# Get recently accessed issues
issues_json=$(run_cmd jira issue list --history --paginate 8 --raw)

if [[ -z "$issues_json" || "$issues_json" == "null" ]]; then
  log_debug "No issues returned"
  echo "Jira | color=gray"
  echo "---"
  echo "No recent issues"
  exit 0
fi

# Parse issues
# Fields: key, summary, status, priority
issue_count=$(echo "$issues_json" | jq -r '.[] | length' 2>/dev/null | wc -l | tr -d ' ')
log_debug "Issue count: $issue_count"

# Helper to echo and cache
cache_output=""
out() {
  echo "$@"
  cache_output+="$*"$'\n'
}

# Menu bar icon
out "📋 | color=blue"
out "---"

# Get Jira base URL from config (required - no default)
jira_url=$(grep -E "^server:" ~/.config/.jira/.config.yml 2>/dev/null | awk '{print $2}' | tr -d '"')
if [[ -z "$jira_url" ]]; then
  log_debug "Jira URL not configured in ~/.config/.jira/.config.yml"
  echo "Jira | color=red"
  echo "---"
  echo "Jira URL not configured"
  echo "Run: jira init"
  exit 0
fi
log_debug "Jira URL: $jira_url"

# Parse issues into two groups: active and closed
declare -a active_issues=()
declare -a closed_issues=()

while IFS=$'\t' read -r key summary status priority; do
  [[ -z "$key" ]] && continue

  # Truncate and sanitize summary
  short_summary="${summary:0:45}"
  [[ ${#summary} -gt 45 ]] && short_summary="${short_summary}..."
  safe_summary="${short_summary//|/-}"

  # Determine color based on status
  case "$status" in
    "Done"|"Closed"|"Resolved")
      color="green"
      icon="checkmark.circle.fill"
      closed_issues+=("$key"$'\t'"$safe_summary"$'\t'"$status"$'\t'"$color"$'\t'"$icon")
      ;;
    "In Progress"|"In Review"|"Work in Progress")
      color="blue"
      icon="arrow.right.circle.fill"
      active_issues+=("$key"$'\t'"$safe_summary"$'\t'"$status"$'\t'"$color"$'\t'"$icon")
      ;;
    "Blocked"|"On Hold")
      color="red"
      icon="xmark.circle.fill"
      active_issues+=("$key"$'\t'"$safe_summary"$'\t'"$status"$'\t'"$color"$'\t'"$icon")
      ;;
    *)
      color="gray"
      icon="circle.fill"
      active_issues+=("$key"$'\t'"$safe_summary"$'\t'"$status"$'\t'"$color"$'\t'"$icon")
      ;;
  esac
done < <(echo "$issues_json" | jq -r '.[] | [.key, .fields.summary, .fields.status.name, (.fields.priority.name // "None")] | @tsv')

# Display active issues first
for issue in "${active_issues[@]}"; do
  IFS=$'\t' read -r key summary status color icon <<< "$issue"
  out "$key: $summary | image=SF:$icon color=$color href=${jira_url}/browse/${key}"
  out "-- Status: $status | color=gray size=11"
done

# Separator and closed issues
if [[ ${#closed_issues[@]} -gt 0 ]]; then
  out "---"
  out "Closed/Done"
  for issue in "${closed_issues[@]}"; do
    IFS=$'\t' read -r key summary status color icon <<< "$issue"
    out "$key: $summary | image=SF:$icon color=$color href=${jira_url}/browse/${key}"
    out "-- Status: $status | color=gray size=11"
  done
fi

out "---"
out "Open Jira | href=${jira_url}"
out "Refresh | refresh=true"

# Save output to cache for idle mode
echo -n "$cache_output" > "$CACHE_FILE"
log_debug "========== Script finished =========="
