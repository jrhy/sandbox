package scriptstest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupMacOSScriptStartsPostgresAndExecsOpenbrain(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	logPath := filepath.Join(tempDir, "startup.log")
	stateDir := filepath.Join(tempDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	containerScript := `#!/usr/bin/env bash
set -euo pipefail
printf 'container %s\n' "$*" >> "$STARTUP_LOG"

state_dir="${STARTUP_STATE_DIR}"
cmd="${1:-}"
shift || true

case "$cmd" in
  system)
    if [[ "${1:-}" == "start" ]]; then
      exit 0
    fi
    ;;
  start)
    if [[ -f "$state_dir/postgres-exists" ]]; then
      touch "$state_dir/postgres-running"
      exit 0
    fi
    exit 1
    ;;
  run)
    touch "$state_dir/postgres-exists" "$state_dir/postgres-running"
    exit 0
    ;;
  exec)
    args=("$@")
    if [[ "${args[0]:-}" == "--interactive" ]]; then
      args=("${args[@]:1}")
    fi
    name="${args[0]:-}"
    shift_count=1
    if [[ "$name" == "" ]]; then
      exit 1
    fi
    if [[ "${args[1]:-}" == "pg_isready" ]]; then
      if [[ -f "$state_dir/postgres-running" ]]; then
        exit 0
      fi
      exit 1
    fi
    if [[ "${args[1]:-}" == "psql" ]]; then
      psql_args="${args[*]}"
      if [[ "$psql_args" == *"-tAc"* ]]; then
        if [[ -f "$state_dir/schema-ready" ]]; then
          printf '1\n'
        fi
        exit 0
      fi
      cat >/dev/null
      touch "$state_dir/schema-ready"
      exit 0
    fi
    ;;
esac

printf 'unexpected container invocation: %s %s\n' "$cmd" "$*" >&2
exit 1
`
	writeExecutable(t, filepath.Join(binDir, "container"), containerScript)

	openbrainScript := `#!/usr/bin/env bash
set -euo pipefail
printf 'openbrain args=%s\n' "$*" >> "$STARTUP_LOG"
printf 'OPENBRAIN_DATABASE_URL=%s\n' "${OPENBRAIN_DATABASE_URL:-}" >> "$STARTUP_LOG"
printf 'OPENBRAIN_OLLAMA_URL=%s\n' "${OPENBRAIN_OLLAMA_URL:-}" >> "$STARTUP_LOG"
printf 'OPENBRAIN_CSRF_KEY=%s\n' "${OPENBRAIN_CSRF_KEY:-}" >> "$STARTUP_LOG"
if [[ "${1:-}" != "start" ]]; then
  echo "expected openbrain start" >&2
  exit 1
fi
`
	writeExecutable(t, filepath.Join(binDir, "openbrain"), openbrainScript)

	curlScript := `#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >> "$STARTUP_LOG"
for arg in "$@"; do
  if [[ "$arg" == */api/version ]]; then
    printf '{"version":"test"}\n'
    exit 0
  fi
done
exit 1
`
	writeExecutable(t, filepath.Join(binDir, "curl"), curlScript)

	cmd := exec.Command("bash", "scripts/startup-macos.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin",
		"OPENBRAIN_CONTAINER_BIN="+filepath.Join(binDir, "container"),
		"OPENBRAIN_CSRF_KEY_FILE="+filepath.Join(tempDir, "csrf.key"),
		"STARTUP_LOG="+logPath,
		"STARTUP_STATE_DIR="+stateDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("startup-macos.sh failed: %v\n%s", err, output)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(rawLog)
	for _, want := range []string{
		"container system start",
		"container run --detach --name openbrain-postgres",
		"container exec openbrain-postgres pg_isready -U openbrain -d openbrain",
		"container exec --interactive openbrain-postgres psql -v ON_ERROR_STOP=1 -U openbrain -d openbrain",
		"curl -fsS http://127.0.0.1:11434/api/version",
		"openbrain args=start",
		"OPENBRAIN_DATABASE_URL=postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable",
		"OPENBRAIN_OLLAMA_URL=http://127.0.0.1:11434",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("startup log missing %q:\n%s", want, logText)
		}
	}
	if strings.Contains(logText, "user update") {
		t.Fatalf("startup log should not include provisioning steps:\n%s", logText)
	}
}

func TestStartupMacOSScriptDoesNotUseWalkthroughProvisioning(t *testing.T) {
	repo := repoRoot(t)
	script, err := os.ReadFile(filepath.Join(repo, "scripts", "startup-macos.sh"))
	if err != nil {
		t.Fatalf("read startup-macos.sh: %v", err)
	}
	text := string(script)
	for _, forbidden := range []string{
		"walkthrough",
		"user update",
		"token create",
		"drop table",
		"podman-compose",
		"docker compose",
		"go run ./cmd/openbrain",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("startup-macos.sh should not contain %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "/usr/local/bin/container") {
		t.Fatalf("startup-macos.sh should default to the macOS container runtime:\n%s", text)
	}
	if !strings.Contains(text, "exec openbrain start") {
		t.Fatalf("startup-macos.sh should exec the compiled openbrain binary:\n%s", text)
	}
}
