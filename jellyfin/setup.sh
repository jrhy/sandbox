#!/usr/bin/env bash
# setup.sh — Install a per-user, sudo-free Jellyfin media server on
# Bazzite / Fedora Atomic (immutable) systems.
#
# Method: rootless Podman container managed by a systemd --user unit.
# No system packages, no root, no /usr overlays. Everything lives in
# your home directory; removing it means deleting the unit + data dir.
#
# Override any default via environment variables:
#   JF_PORT             HTTP port exposed on the host (default 8096)
#   JF_MEDIA_DIR        Host media directory (default /var/mnt/hdd/media)
#   JF_ADMIN_USER       Admin username created in first-run wizard (default admin)
#   JF_ADMIN_PASSWORD   Admin password (default: generated, saved to
#                       $JF_DATA_DIR/ADMIN_CREDENTIALS, mode 600)
#   JF_LIBRARY_NAME     Library display name (default Movies)
#   JF_IMAGE            Container image (default docker.io/jellyfin/jellyfin:latest)
#
# The script is idempotent: re-run it to repair or reconfigure.
set -euo pipefail

# --------------------------------------------------------------- defaults
JF_PORT="${JF_PORT:-8096}"
JF_MEDIA_DIR="${JF_MEDIA_DIR:-/var/mnt/hdd/media}"
JF_ADMIN_USER="${JF_ADMIN_USER:-admin}"
JF_ADMIN_PASSWORD="${JF_ADMIN_PASSWORD:-}"
JF_LIBRARY_NAME="${JF_LIBRARY_NAME:-Movies}"
JF_IMAGE="${JF_IMAGE:-docker.io/jellyfin/jellyfin:latest}"
JF_LIBRARY_TYPE="${JF_LIBRARY_TYPE:-movies}"   # movies / tvshows / music / ...

JF_DATA_DIR="${JF_DATA_DIR:-$HOME/.local/share/jellyfin}"
JF_UNIT="$HOME/.config/systemd/user/container-jellyfin.service"
B="http://127.0.0.1:${JF_PORT}"

# NOTE: on Bazzite / Silverblue-style systems /mnt is a symlink to
# /var/mnt, and rootless podman cannot bind-mount through it. Resolve
# the real path first. (Plain readlink -f would also follow this.)
JF_MEDIA_DIR=$(readlink -f "$JF_MEDIA_DIR")

die() { echo "SETUP FAILED: $*" >&2; exit 1; }
note() { printf '\n== %s\n' "$*"; }

command -v podman >/dev/null || die "podman not found (it ships with Bazzite)"
[ -d "$JF_MEDIA_DIR" ] || die "media dir '$JF_MEDIA_DIR' does not exist"

# --------------------------------------------------------------- state dir
note "Creating data dirs under $JF_DATA_DIR"
mkdir -p "$JF_DATA_DIR/config" "$JF_DATA_DIR/cache" \
         "$(dirname "$JF_UNIT")"

# --------------------------------------------------------------- unit file
# --security-opt label=disable: avoids SELinux relabeling of the
#   (possibly root-owned, shared) media directory. The mount is :ro.
# --replace: makes restarts robust against name collisions.
note "Writing systemd user unit: $JF_UNIT"
cat > "$JF_UNIT" <<EOF
[Unit]
Description=Jellyfin media server (rootless podman container)
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/bin/podman run --rm --replace --name jellyfin \\
    -p ${JF_PORT}:8096 -p 8920:8920 \\
    -v ${JF_DATA_DIR}/config:/config \\
    -v ${JF_DATA_DIR}/cache:/cache \\
    -v ${JF_MEDIA_DIR}:/media:ro \\
    --security-opt label=disable \\
    ${JF_IMAGE}
ExecStop=/usr/bin/podman stop -t 30 jellyfin

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload

# --------------------------------------------------------------- container
note "Pulling image: $JF_IMAGE"
podman pull "$JF_IMAGE"

note "Enabling + starting service"
systemctl --user enable --now container-jellyfin.service

# --------------------------------------------------------------- helper API functions
http_code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

api() { # api METHOD PATH [JSON_BODY] [TOKEN]
    local m=$1 p=$2 body=${3:-} token=${4:-}
    local args=(-s -X "$m" "$B$p" -H 'Content-Type: application/json')
    [ -n "$token" ] && args+=(-H "X-Emby-Token: $token")
    [ -n "$body" ] && args+=(-d "$body")
    curl "${args[@]}"
}

JF_AUTH='MediaBrowser Client="jellyfin-setup", Device="shell", DeviceId="jellyfin-setup", Version="1.0"'

wait_for_server() {
    note "Waiting for Jellyfin to come up on $B"
    for _ in $(seq 1 60); do
        [ "$(http_code "$B/System/Info/Public")" = 200 ] && return 0
        sleep 2
    done
    die "server did not come up; check: journalctl --user -u container-jellyfin"
}

# --------------------------------------------------------------- start server
wait_for_server

# --------------------------------------------------------------- admin credentials
if [ -z "$JF_ADMIN_PASSWORD" ]; then
    # reuse a previously saved password (idempotent re-runs)
    if [ -f "$JF_DATA_DIR/ADMIN_CREDENTIALS" ]; then
        JF_ADMIN_PASSWORD=$(sed -n 2p "$JF_DATA_DIR/ADMIN_CREDENTIALS")
    else
        JF_ADMIN_PASSWORD=$(head -c 12 /dev/urandom | base64 | tr -d '/+=' | head -c 16)
    fi
fi
printf '%s\n%s\n' "$JF_ADMIN_USER" "$JF_ADMIN_PASSWORD" > "$JF_DATA_DIR/ADMIN_CREDENTIALS"
chmod 600 "$JF_DATA_DIR/ADMIN_CREDENTIALS"

# --------------------------------------------------------------- first-run wizard
WIZARD=$(curl -s --max-time 10 "$B/System/Info/Public" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("StartupWizardCompleted"))')
if [ "$WIZARD" != "True" ]; then
    note "Completing first-run wizard (creates admin user '$JF_ADMIN_USER')"
    # Order matters in Jellyfin 10.11: GET /Startup/User initializes the
    # pending wizard user (default name "root"); only then may you POST
    # your chosen name/password. POSTing first returns 404.
    # First boot also runs DB migrations - endpoints can be briefly
    # flaky, hence the retries.
    for _ in $(seq 1 30); do
        [ "$(http_code "$B/Startup/User")" = 200 ] && break
        sleep 2
    done
    # POST the admin credentials until it succeeds (204)
    for _ in $(seq 1 10); do
        [ "$(http_code -X POST "$B/Startup/User" -H 'Content-Type: application/json' \
            -d "{\"Name\":\"$JF_ADMIN_USER\",\"Password\":\"$JF_ADMIN_PASSWORD\"}")" = 204 ] && break
        sleep 2
    done
    # complete the wizard (204 on success), retrying transient failures
    for _ in $(seq 1 10); do
        if http_code -X POST "$B/Startup/Complete" | grep -q 204; then
            break
        fi
        sleep 2
    done
    # confirm the wizard actually completed
    [ "$(curl -s "$B/System/Info/Public" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("StartupWizardCompleted"))')" = True ] \
        || die "wizard did not complete"
else
    note "Wizard already complete"
fi

# --------------------------------------------------------------- authenticate
# Note: the X-Emby-Authorization header is required by this endpoint
# even for the login request itself.
TOKEN=$(curl -s --max-time 10 -X POST "$B/Users/AuthenticateByName" \
    -H 'Content-Type: application/json' -H "X-Emby-Authorization: $JF_AUTH" \
    -d "{\"Username\":\"$JF_ADMIN_USER\",\"Pw\":\"$JF_ADMIN_PASSWORD\"}" \
    | python3 -c 'import json,sys;print(json.load(sys.stdin).get("AccessToken",""))' 2>/dev/null)
[ -n "$TOKEN" ] \
    || die "authentication failed for '$JF_ADMIN_USER' (wrong password? rerun with JF_ADMIN_PASSWORD=...)"
note "Authenticated as $JF_ADMIN_USER"

# --------------------------------------------------------------- remote access
# Enabled by default for LAN clients. Disable with JF_ALLOW_REMOTE=0.
if [ "${JF_ALLOW_REMOTE:-1}" = 1 ]; then
    note "Enabling remote access (LAN clients)"
    curl -s "$B/System/Configuration/network" -H "X-Emby-Token: $TOKEN" -o /tmp/jf-netcfg.json
    python3 -c '
import json
d = json.load(open("/tmp/jf-netcfg.json"))
d["EnableRemoteAccess"] = True
json.dump(d, open("/tmp/jf-netcfg-out.json", "w"))'
    curl -s -o /dev/null -X POST "$B/System/Configuration/network" \
        -H "X-Emby-Token: $TOKEN" -H 'Content-Type: application/json' \
        -d @/tmp/jf-netcfg-out.json
fi

# --------------------------------------------------------------- media library
LIBS=$(api GET /Library/VirtualFolders "" "$TOKEN")
EXISTS=$(echo "$LIBS" | python3 -c '
import json,sys
folders = json.load(sys.stdin)
print(any(f["Name"] == sys.argv[1] for f in folders))' "$JF_LIBRARY_NAME" 2>/dev/null || echo False)
if [ "$EXISTS" = "True" ]; then
    note "Library '$JF_LIBRARY_NAME' already exists"
else
    note "Creating library '$JF_LIBRARY_NAME' from $JF_MEDIA_DIR"
    # Paths inside the container are prefixed with /media (see volume mount).
    curl -s -o /dev/null -X POST \
        "$B/Library/VirtualFolders?name=${JF_LIBRARY_NAME}&collectionType=${JF_LIBRARY_TYPE}&refreshLibrary=false" \
        -H "X-Emby-Token: $TOKEN" -H 'Content-Type: application/json' \
        -d '{"LibraryOptions":{}}'
    curl -s -o /dev/null -X POST "$B/Library/VirtualFolders/Paths?refreshLibrary=false" \
        -H "X-Emby-Token: $TOKEN" -H 'Content-Type: application/json' \
        -d "{\"Name\":\"$JF_LIBRARY_NAME\",\"PathInfo\":{\"Path\":\"/media\"}}"
fi

note "Scanning library (this may take a while)"
curl -s -o /dev/null -X POST "$B/Library/Refresh" -H "X-Emby-Token: $TOKEN"

COUNT=0
for _ in $(seq 1 60); do
    sleep 5
    COUNT=$(api GET "/Items?recursive=true&fields=Path" "" "$TOKEN" \
        | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d.get("TotalRecordCount",0))' 2>/dev/null || echo 0)
    [ "${COUNT:-0}" -gt 0 ] && break
done
note "Library scan finished: $COUNT item(s) indexed"

# --------------------------------------------------------------- linger (start at boot)
if ! loginctl show-user "$USER" -p Linger 2>/dev/null | grep -q '^Linger=yes'; then
    if loginctl enable-linger 2>/dev/null; then
        note "Linger enabled: services start at boot without login"
    else
        echo "NOTE: could not enable linger; server starts when you log in"
    fi
fi

# --------------------------------------------------------------- summary
LAN_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
cat <<EOF

================ Jellyfin setup complete ================
  Web UI (local):      http://127.0.0.1:${JF_PORT}
  Web UI (LAN):       http://${LAN_IP:-<lan-ip>}:${JF_PORT}
  Admin user:         $JF_ADMIN_USER
  Admin password:     in $JF_DATA_DIR/ADMIN_CREDENTIALS (mode 600)
  Media (read-only):  $JF_MEDIA_DIR -> /media in container
  Service:            systemctl --user status container-jellyfin

  Items indexed:      $COUNT

Caveats:
  - If LAN clients cannot connect, your distro firewall may block
    port ${JF_PORT}; on Bazzite check Firewall settings in the
    desktop settings panel (usually allowed for home LANs).
  - Auto-discovery (UDP 7359) is not proxied by rootless podman;
    add the server manually by IP in clients.
  - Media is mounted read-only; Jellyfin writes metadata/art to
    $JF_DATA_DIR/config instead.
=========================================================
EOF

# --------------------------------------------------------------- verify
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
[ -x "$SCRIPT_DIR/verify.sh" ] && "$SCRIPT_DIR/verify.sh"
