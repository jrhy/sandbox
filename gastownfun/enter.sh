#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
CONTAINER_NAME=${GASTOWNFUN_CONTAINER_NAME:-gastownfun-alpine}
TARGET_USER=${1:-user}

case "$TARGET_USER" in
  user)
    HOME_DIR=/data/user
    ;;
  root)
    HOME_DIR=/data/root
    ;;
  *)
    printf 'Usage: %s [user|root]\n' "$0" >&2
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

if ! podman container exists "$CONTAINER_NAME"; then
  "$SCRIPT_DIR/ensure-container.sh"
elif [ "$(podman inspect --format '{{.State.Running}}' "$CONTAINER_NAME")" != "true" ]; then
  podman start "$CONTAINER_NAME" >/dev/null
fi

exec podman exec -it \
  -u "$TARGET_USER" \
  -e "HOME=$HOME_DIR" \
  -e "USER=$TARGET_USER" \
  -e "LOGNAME=$TARGET_USER" \
  -e 'SHELL=/bin/bash' \
  -w "$HOME_DIR" \
  "$CONTAINER_NAME" \
  /bin/bash -l
