#!/bin/sh
set -eu

BASE_DIR="$(pwd)"
BASE_DIR_REAL="$(cd "$BASE_DIR" && pwd -P)"
USER_HOME="${HOME:-}"
if [ -z "$USER_HOME" ] || [ "${USER_HOME#/Users/}" = "$USER_HOME" ]; then
  USER_HOME="/Users/$(id -un)"
fi
TMP_BASE="${TMPDIR:-/tmp}"
TMP_DIR="$(mktemp -d "${TMP_BASE%/}/sandbox-tmp.XXXXXX")"
PROFILE="$(mktemp "${TMP_DIR}/sandbox.profile.XXXXXX")"
HOME_DIR="$(mktemp -d "${TMP_DIR}/sandbox-home.XXXXXX")"
trap 'rm -f "$PROFILE"; rm -rf "$TMP_DIR"; rm -rf "$HOME_DIR"' EXIT

PATH_RULES=""
PARENT_RULES=""
USER_DENY_RULES=""
OLD_IFS="$IFS"
IFS=:
for path_dir in $PATH; do
  if [ -n "$path_dir" ]; then
    if [ "$path_dir" = "$USER_HOME" ]; then
      continue
    fi
    PATH_RULES="$PATH_RULES
(allow file-read* (subpath \"$path_dir\"))
(allow file-map-executable (subpath \"$path_dir\"))"
  fi
done
IFS="$OLD_IFS"

escape_regex() {
  printf '%s' "$1" | sed 's/[.[\\^$*+?()|{}]/\\\\&/g'
}

user_prefixes=""
add_user_prefix() {
  case "$1" in
    /Users/*)
      if [ "$1" = "$USER_HOME" ]; then
        return
      fi
      rel="${1#/Users/}"
      if [ -n "$rel" ]; then
        esc="$(escape_regex "$rel")"
        if [ -z "$user_prefixes" ]; then
          user_prefixes="$esc"
        else
          user_prefixes="$user_prefixes|$esc"
        fi
      fi
      ;;
  esac
}

add_user_prefix "$BASE_DIR"
add_user_prefix "$BASE_DIR_REAL"
OLD_IFS="$IFS"
IFS=:
for path_dir in $PATH; do
  add_user_prefix "$path_dir"
done
IFS="$OLD_IFS"

if [ -z "$user_prefixes" ]; then
  user_prefixes="__none__"
fi

USER_DENY_RULES="$USER_DENY_RULES
(allow file-read-metadata (subpath \"/Users\"))
(allow file-read-metadata (subpath \"/System/Volumes/Data/Users\"))
(deny file-read-data (regex #\"^/Users/(?!($user_prefixes)(/|$)).*\"))
(deny file-read-data (regex #\"^/System/Volumes/Data/Users/(?!($user_prefixes)(/|$)).*\"))
(deny file-map-executable (regex #\"^/Users/(?!($user_prefixes)(/|$)).*\"))
(deny file-map-executable (regex #\"^/System/Volumes/Data/Users/(?!($user_prefixes)(/|$)).*\"))"

PARENT_DIR="$BASE_DIR"
while [ "$PARENT_DIR" != "/" ]; do
  PARENT_DIR="$(dirname "$PARENT_DIR")"
  if [ "$PARENT_DIR" != "/" ]; then
    PARENT_RULES="$PARENT_RULES
(allow file-read-metadata (literal \"$PARENT_DIR\"))
(deny file-read-data (literal \"$PARENT_DIR\"))"
  fi
done

cat > "$PROFILE" <<PROFILE_EOF
(version 1)
(deny default)

(allow process*)
(allow sysctl-read)
(allow mach-lookup)

(allow file-read* (subpath "/System") (subpath "/usr") (subpath "/Library") (subpath "/System/Volumes/Data/Library") (subpath "/private") (subpath "/etc") (subpath "/dev") (subpath "/bin") (subpath "/sbin"))
(allow file-map-executable (subpath "/System") (subpath "/usr") (subpath "/Library") (subpath "/System/Volumes/Data/Library") (subpath "/bin") (subpath "/sbin"))
(allow file-read-metadata (subpath "/System/Cryptexes/App") (subpath "/System/Cryptexes/OS"))
(allow file-write* (literal "/dev/null") (literal "/dev/tty") (literal "/dev/fd"))
(allow file-read-data (literal "/"))
(allow file-read-metadata (literal "/"))
(allow file-read-metadata (subpath "/var"))
(allow file-ioctl (subpath "/dev"))
(allow file-read* (subpath "$BASE_DIR"))
(allow file-read* (subpath "$BASE_DIR_REAL"))
(allow file-write* (subpath "$BASE_DIR"))
(allow file-read* (subpath "$TMP_DIR"))
(allow file-write* (subpath "$TMP_DIR"))
$PATH_RULES
$PARENT_RULES
$USER_DENY_RULES
PROFILE_EOF

if [ "$#" -eq 0 ]; then
  cat >&2 <<USAGE_EOF
usage: $0 <command> [args...]

Runs a command under sandbox-exec with filesystem access limited to the
current directory (plus read-only system/runtime paths).
USAGE_EOF
  exit 2
fi

HOME="$HOME_DIR" TMPDIR="$TMP_DIR" TMP="$TMP_DIR" TEMP="$TMP_DIR" \
  /usr/bin/sandbox-exec -f "$PROFILE" "$@"
