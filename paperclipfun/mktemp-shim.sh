#!/bin/sh
# Installed as "mktemp" on an agent's PATH by paperclip-sb-claude.
# macOS mktemp ignores $TMPDIR for bare templates and for -t: both use the
# per-user temp dir, which is outside the sandbox. Rewrite those two forms onto
# $TMPDIR; anything that already names a directory passes through untouched.
dir="${TMPDIR:-/tmp}"
seen_dir=
args=
while [ $# -gt 0 ]; do
  a=$1; shift
  case $a in
    -t) p=$1; shift; args="$args '$dir/$p.XXXXXXXXXX'"; seen_dir=1 ;;
    -p|--tmpdir|--tmpdir=*|-p?*|/*|*/*) args="$args '$a'"; seen_dir=1 ;;
    *) args="$args '$a'" ;;
  esac
done
[ -n "$seen_dir" ] || args="-p '$dir' $args"
eval "exec /usr/bin/mktemp $args"
