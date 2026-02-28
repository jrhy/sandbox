#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
DATA_DIR=$SCRIPT_DIR/data
CONTAINER_NAME=${GASTOWNFUN_CONTAINER_NAME:-gastownfun-alpine}
IMAGE=${GASTOWNFUN_IMAGE:-localhost/gastownfun:latest}
BASE_IMAGE=${GASTOWNFUN_BASE_IMAGE:-docker.io/library/alpine:latest}
RECREATE=0

usage() {
  printf 'Usage: %s [--recreate]\n' "$0" >&2
}

case "${1-}" in
  "")
    ;;
  --reset|--recreate)
    RECREATE=1
    ;;
  *)
    usage
    exit 1
    ;;
esac

if ! command -v podman >/dev/null 2>&1; then
  printf 'podman was not found in PATH.\n' >&2
  exit 1
fi

if ! podman info >/dev/null 2>&1; then
  podman machine start >/dev/null
fi

ARCH=$(podman info --format '{{.Host.Arch}}')
case "$ARCH" in
  aarch64)
    ARCH=arm64
    ;;
  x86_64)
    ARCH=amd64
    ;;
esac
PLATFORM=${GASTOWNFUN_PLATFORM:-linux/$ARCH}

mkdir -p "$DATA_DIR" "$DATA_DIR/root" "$DATA_DIR/user"

printf 'Building image %s for %s...\n' "$IMAGE" "$PLATFORM"
podman build \
  --platform "$PLATFORM" \
  --build-arg "BASE_IMAGE=$BASE_IMAGE" \
  --tag "$IMAGE" \
  --file "$SCRIPT_DIR/Containerfile" \
  "$SCRIPT_DIR"

if [ "$RECREATE" -eq 1 ] && podman container exists "$CONTAINER_NAME"; then
  if [ "$(podman inspect --format '{{.State.Running}}' "$CONTAINER_NAME")" = "true" ]; then
    podman stop "$CONTAINER_NAME" >/dev/null
  fi
  podman rm "$CONTAINER_NAME" >/dev/null
fi

if ! podman container exists "$CONTAINER_NAME"; then
  podman create \
    --name "$CONTAINER_NAME" \
    --hostname gastownfun \
    --volume "$DATA_DIR:/data" \
    "$IMAGE" >/dev/null
  printf 'Created container %s using %s.\n' "$CONTAINER_NAME" "$IMAGE"
else
  printf 'Keeping existing container %s; use --recreate to apply new image layers.\n' "$CONTAINER_NAME"
fi

if [ "$(podman inspect --format '{{.State.Running}}' "$CONTAINER_NAME")" != "true" ]; then
  podman start "$CONTAINER_NAME" >/dev/null
fi

podman exec -i -e HOME=/data/root "$CONTAINER_NAME" sh <<'EOF'
set -eu

mkdir -p /data/root /data/user
chmod 700 /data/root /data/user
chown 0:0 /data/root
chown user:user /data/user

mkdir -p /data/user/.codex
CONFIG=/data/user/.codex/config.toml
tmp=$(mktemp)
{
  printf 'project_doc_fallback_filenames = ["CLAUDE.md"]\n'
  if [ -f "$CONFIG" ] && [ -s "$CONFIG" ]; then
    printf '\n'
    grep -Ev '^[[:space:]]*project_doc_fallback_filenames[[:space:]]*=' "$CONFIG" || true
  fi
} >"$tmp"
mv "$tmp" "$CONFIG"
chown -R user:user /data/user/.codex

command -v dolt >/dev/null 2>&1
command -v bd >/dev/null 2>&1
command -v gt >/dev/null 2>&1
command -v codex >/dev/null 2>&1
command -v pgrep >/dev/null 2>&1
ps -p 1 -o command= >/dev/null 2>&1
test "$(readlink -f "$(command -v lsof)")" != "/bin/busybox"
EOF

printf 'Ensured %s from image %s.\n' "$CONTAINER_NAME" "$IMAGE"
