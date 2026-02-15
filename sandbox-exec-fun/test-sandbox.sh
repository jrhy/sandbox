#!/bin/sh
set -eu

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"
WRAP="$BASE_DIR/sandbox-wrap.sh"
PYTHON="${PYTHON:-}"
if [ -z "$PYTHON" ]; then
  PYTHON="$(command -v python3 2>/dev/null || true)"
fi
if [ -z "$PYTHON" ]; then
  echo "python3 not found; set PYTHON=/path/to/python3" >&2
  exit 1
fi
USER_DIR="/Users/$(id -un)"
TEST_ROOT="$(mktemp -d "$USER_DIR/sandbox-exec-fun-test.XXXXXX")"
BIN_DIR="$TEST_ROOT/bin"

trap 'rm -rf "$TEST_ROOT"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_ok() {
  label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    echo "ok - $label"
  else
    fail "$label"
  fi
}

assert_fail() {
  label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "$label (unexpected success)"
  else
    echo "ok - $label"
  fi
}

# 1) Python example runs and enforces outside/网络 restrictions
assert_ok "python example" \
  env PYTHONDONTWRITEBYTECODE=1 PYTHONNOUSERSITE=1 \
  "$WRAP" "$PYTHON" -S "$BASE_DIR/example.py"

# 2) ls .. should be blocked
assert_fail "ls .. blocked" \
  "$WRAP" /bin/ls ..

# 3) direct read from home should be blocked
assert_fail "read from /Users blocked" \
  "$WRAP" /bin/cat "$HOME/.CFUserTextEncoding"

# 4) reading inside current directory should work
assert_ok "read current dir" \
  "$WRAP" /bin/cat "$BASE_DIR/README.md"

# 5) writing via symlinked cwd should work
SYMLINK_DIR="$(mktemp -d "$USER_DIR/sandbox-exec-fun-symlink.XXXXXX")"
SYMLINK_PATH="$SYMLINK_DIR/cwd-link"
ln -s "$BASE_DIR" "$SYMLINK_PATH"
assert_ok "write via symlinked cwd" \
  /bin/sh -c "cd \"$SYMLINK_PATH\" && \"$WRAP\" /bin/sh -c 'echo ok > symlink.txt'"
rm -rf "$SYMLINK_DIR"

# 6) network access should be blocked (localhost)
assert_fail "network blocked" \
  "$WRAP" /bin/sh -c 'python3 -c "import socket; socket.create_connection((\"127.0.0.1\", 80), timeout=1)"'

# 6) /Users access is blocked except for PATH entries
mkdir -p "$BIN_DIR"
cat > "$BIN_DIR/hello-path" <<'SCRIPT_EOF'
#!/bin/sh
echo "hello from PATH"
SCRIPT_EOF
chmod +x "$BIN_DIR/hello-path"
echo "secret" > "$TEST_ROOT/secret.txt"

assert_ok "PATH allowlist in /Users works" \
  env PATH="$BIN_DIR:/usr/bin:/bin" \
  "$WRAP" hello-path

assert_fail "non-PATH /Users read blocked" \
  "$WRAP" /bin/cat "$TEST_ROOT/secret.txt"

# 7) git repo outside current directory is blocked
mkdir -p "$TEST_ROOT/repo"
git init -q "$TEST_ROOT/repo"
echo "hi" > "$TEST_ROOT/repo/README.md"
git -C "$TEST_ROOT/repo" add README.md
git -C "$TEST_ROOT/repo" -c user.name="sandbox" -c user.email="sandbox@example.invalid" \
  commit -q -m "init"

assert_fail "git repo outside current dir blocked" \
  "$WRAP" /usr/bin/git -C "$TEST_ROOT/repo" status -sb

echo "all tests passed"
