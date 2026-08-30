#!/usr/bin/env bash
# verify.sh — End-to-end health check for the per-user Jellyfin setup.
#
# Checks: service running, HTTP API up, admin login works, media is
# indexed, and an actual video stream serves bytes.
#
# Usage:   ./verify.sh
# Env:     JF_PORT (default 8096)
#          JF_DATA_DIR (default ~/.local/share/jellyfin) — reads
#          ADMIN_CREDENTIALS (line1: user, line2: password)
#          JF_ADMIN_USER / JF_ADMIN_PASSWORD (override the file)
set -uo pipefail

JF_PORT="${JF_PORT:-8096}"
JF_DATA_DIR="${JF_DATA_DIR:-$HOME/.local/share/jellyfin}"
B="http://127.0.0.1:${JF_PORT}"
AUTH='MediaBrowser Client="jellyfin-verify", Device="shell", DeviceId="jellyfin-verify", Version="1.0"'

USER="${JF_ADMIN_USER:-$(sed -n 1p "$JF_DATA_DIR/ADMIN_CREDENTIALS" 2>/dev/null)}"
PASS="${JF_ADMIN_PASSWORD:-$(sed -n 2p "$JF_DATA_DIR/ADMIN_CREDENTIALS" 2>/dev/null)}"

PASS_CNT=0; FAIL_CNT=0
ok()   { printf 'PASS  %s\n' "$1"; PASS_CNT=$((PASS_CNT+1)); }
bad()  { printf 'FAIL  %s\n' "$1"; FAIL_CNT=$((FAIL_CNT+1)); }
check(){ "$@" >/dev/null 2>&1 && ok "$1" || bad "$1"; }

# 1. systemd user service active
[ "$(systemctl --user is-active container-jellyfin.service)" = active ] \
    && ok "systemd user service active" || bad "systemd user service active"

# 2. container actually running
podman ps --format '{{.Names}}' | grep -qx jellyfin \
    && ok "podman container running" || bad "podman container running"

# 3. HTTP API reachable (poll briefly — the server may still be starting
#    right after a container restart)
INFO=""
for _ in $(seq 1 15); do
    INFO=$(curl -s --max-time 10 "$B/System/Info/Public")
    echo "$INFO" | python3 -c 'import json,sys;json.load(sys.stdin)' 2>/dev/null && break
    INFO=""
    sleep 2
done
if [ -n "$INFO" ]; then
    VER=$(echo "$INFO" | python3 -c 'import json,sys;print(json.load(sys.stdin)["Version"])')
    ok "HTTP API reachable (Jellyfin $VER)"
else
    bad "HTTP API reachable"
    VER=""
fi

# 4. admin authentication
TOKEN=""
if [ -n "$USER" ] && [ -n "$PASS" ]; then
    TOKEN=$(curl -s --max-time 10 -X POST "$B/Users/AuthenticateByName" \
        -H 'Content-Type: application/json' -H "X-Emby-Authorization: $AUTH" \
        -d "{\"Username\":\"$USER\",\"Pw\":\"$PASS\"}" \
        | python3 -c 'import json,sys;print(json.load(sys.stdin).get("AccessToken",""))' 2>/dev/null)
fi
[ -n "$TOKEN" ] \
    && ok "admin login as '$USER'" || bad "admin login as '$USER'"

# 5. media indexed (any item in the library)
COUNT=$(curl -s --max-time 10 "$B/Items?recursive=true" -H "X-Emby-Token: $TOKEN" \
    | python3 -c 'import json,sys;print(json.load(sys.stdin).get("TotalRecordCount",0))' 2>/dev/null || echo 0)
[ "$COUNT" -gt 0 ] \
    && ok "library indexed ($COUNT item(s))" || bad "library indexed (0 items)"

# 6. stream bytes of the first video through Jellyfin
MID=$(curl -s --max-time 10 "$B/Items?recursive=true&includeItemTypes=Movie,Episode,MusicVideo,Audio&fields=Path" \
    -H "X-Emby-Token: $TOKEN" \
    | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["Items"][0]["Id"] if d.get("Items") else "")' 2>/dev/null)
if [ -n "$MID" ]; then
    BYTES=$(curl -s --max-time 20 -r 0-262143 \
        "$B/Videos/$MID/stream.mp4?static=true" -H "X-Emby-Token: $TOKEN" \
        -o /dev/null -w '%{size_download}')
    [ "${BYTES:-0}" -gt 0 ] \
        && ok "video stream serves bytes ($BYTES bytes)" || bad "video stream serves bytes"
else
    bad "video stream serves bytes (no playable item found)"
fi

echo "----------------------------------------"
echo "$PASS_CNT passed, $FAIL_CNT failed"
[ "$FAIL_CNT" -eq 0 ] && exit 0 || exit 1
