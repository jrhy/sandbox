package scriptstest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func envWithoutOpenBrainDatabaseURL() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, "OPENBRAIN_DATABASE_URL=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func writeStartupMacOSFakes(t *testing.T, binDir, logPath, stateDir string) {
	t.Helper()

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
      if [[ -f "$state_dir/fail-start" ]]; then
        exit 1
      fi
      if [[ -f "$state_dir/stays-stopped" ]]; then
        touch "$state_dir/postgres-stopped"
        rm -f "$state_dir/postgres-running"
        exit 0
      fi
      touch "$state_dir/postgres-running"
      rm -f "$state_dir/postgres-stopped"
      exit 0
    fi
    exit 1
    ;;
  run)
    touch "$state_dir/postgres-exists"
    if [[ -f "$state_dir/stays-stopped" ]]; then
      touch "$state_dir/postgres-stopped"
      rm -f "$state_dir/postgres-running"
      exit 0
    fi
    touch "$state_dir/postgres-running"
    rm -f "$state_dir/postgres-stopped"
    exit 0
    ;;
  inspect)
    if [[ ! -f "$state_dir/postgres-exists" ]]; then
      printf '[]\n'
      exit 0
    fi
    status="running"
    if [[ -f "$state_dir/postgres-stopped" || ! -f "$state_dir/postgres-running" ]]; then
      status="stopped"
    fi
    printf '[{"status":"%s"}]\n' "$status"
    exit 0
    ;;
  logs)
    if [[ -f "$state_dir/logs.txt" ]]; then
      cat "$state_dir/logs.txt"
    fi
    exit 0
    ;;
  exec)
    args=("$@")
    if [[ "${args[0]:-}" == "--interactive" ]]; then
      args=("${args[@]:1}")
    fi
    name="${args[0]:-}"
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

	_ = logPath
}

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

	writeStartupMacOSFakes(t, binDir, logPath, stateDir)

	cmd := exec.Command("bash", "scripts/startup-macos.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin",
		"OPENBRAIN_ENV_FILE="+filepath.Join(tempDir, "missing.env"),
		"OPENBRAIN_CONTAINER_BIN="+filepath.Join(binDir, "container"),
		"OPENBRAIN_CSRF_KEY=test-csrf-key",
		"OPENBRAIN_DATABASE_URL=postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable",
		"OPENBRAIN_POSTGRES_PASSWORD=openbrain",
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
		"container run --detach --name openbrain-postgres --env POSTGRES_DB=openbrain --env POSTGRES_USER=openbrain --env POSTGRES_PASSWORD=openbrain --env PGDATA=/var/lib/postgresql/data/pgdata --publish 127.0.0.1:5432:5432 --volume openbrain-postgres-data:/var/lib/postgresql/data pgvector/pgvector:pg16",
		"container exec openbrain-postgres pg_isready -U openbrain -d openbrain",
		"container inspect openbrain-postgres",
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

func TestStartupMacOSScriptFailsFastForStoppedExistingContainer(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(stateDir, "postgres-exists"), []byte("1"), 0o644); err != nil {
		t.Fatalf("seed postgres exists: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "stays-stopped"), []byte("1"), 0o644); err != nil {
		t.Fatalf("seed stopped state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "logs.txt"), []byte("chmod: changing permissions of '/var/lib/postgresql/data': Operation not permitted\nchown: changing ownership of '/var/lib/postgresql/data': Operation not permitted\n"), 0o644); err != nil {
		t.Fatalf("seed logs: %v", err)
	}

	writeStartupMacOSFakes(t, binDir, logPath, stateDir)

	cmd := exec.Command("bash", "scripts/startup-macos.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin",
		"OPENBRAIN_ENV_FILE="+filepath.Join(tempDir, "missing.env"),
		"OPENBRAIN_CONTAINER_BIN="+filepath.Join(binDir, "container"),
		"OPENBRAIN_CSRF_KEY=test-csrf-key",
		"STARTUP_LOG="+logPath,
		"STARTUP_STATE_DIR="+stateDir,
		"OPENBRAIN_POSTGRES_READY_TIMEOUT_SECONDS=2",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("startup-macos.sh unexpectedly succeeded:\n%s", output)
	}

	text := string(output)
	for _, want := range []string{
		"entered terminal state: stopped",
		"chmod: changing permissions of '/var/lib/postgresql/data': Operation not permitted",
		"container rm openbrain-postgres",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("startup failure output missing %q:\n%s", want, text)
		}
	}

	rawLog, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read log: %v", readErr)
	}
	logText := string(rawLog)
	if strings.Contains(logText, "container run --detach --name openbrain-postgres") {
		t.Fatalf("startup should not try to recreate an existing container before reporting failure:\n%s", logText)
	}
}

func TestStartupMacOSScriptRequiresCSRFFromEnvOrDotEnv(t *testing.T) {
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

	writeStartupMacOSFakes(t, binDir, logPath, stateDir)

	cmd := exec.Command("bash", "scripts/startup-macos.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin",
		"OPENBRAIN_ENV_FILE="+filepath.Join(tempDir, "missing.env"),
		"OPENBRAIN_CONTAINER_BIN="+filepath.Join(binDir, "container"),
		"STARTUP_LOG="+logPath,
		"STARTUP_STATE_DIR="+stateDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("startup-macos.sh unexpectedly succeeded without OPENBRAIN_CSRF_KEY:\n%s", output)
	}

	text := string(output)
	for _, want := range []string{
		"OPENBRAIN_CSRF_KEY is required",
		"scripts/init-env.sh",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("startup failure output missing %q:\n%s", want, text)
		}
	}

	rawLog, readErr := os.ReadFile(logPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read log: %v", readErr)
	}
	logText := string(rawLog)
	if strings.Contains(logText, "container system start") {
		t.Fatalf("startup should fail before touching the container runtime when csrf is missing:\n%s", logText)
	}
}

func TestStartupMacOSScriptLoadsEnvFile(t *testing.T) {
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

	envFile := filepath.Join(tempDir, ".env")
	envText := strings.Join([]string{
		"OPENBRAIN_POSTGRES_CONTAINER_NAME=env-postgres",
		"OPENBRAIN_POSTGRES_VOLUME_NAME=env-volume",
		"OPENBRAIN_POSTGRES_DB=envdb",
		"OPENBRAIN_POSTGRES_USER=envuser",
		"OPENBRAIN_POSTGRES_PASSWORD=envpass",
		"OPENBRAIN_POSTGRES_HOST_PORT=55432",
		"OPENBRAIN_OLLAMA_URL=http://127.0.0.1:21434",
		"OPENBRAIN_COOKIE_SECURE=true",
		"OPENBRAIN_CSRF_KEY=env-csrf-key",
	}, "\n") + "\n"
	if err := os.WriteFile(envFile, []byte(envText), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	writeStartupMacOSFakes(t, binDir, logPath, stateDir)

	cmd := exec.Command("bash", "scripts/startup-macos.sh")
	cmd.Dir = repo
	cmd.Env = append(envWithoutOpenBrainDatabaseURL(),
		"PATH="+binDir+":/usr/bin:/bin",
		"OPENBRAIN_ENV_FILE="+envFile,
		"OPENBRAIN_CONTAINER_BIN="+filepath.Join(binDir, "container"),
		"STARTUP_LOG="+logPath,
		"STARTUP_STATE_DIR="+stateDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("startup-macos.sh failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "OPENBRAIN_COOKIE_SECURE=true means browser login cookies are only sent over HTTPS") {
		t.Fatalf("startup output missing secure-cookie warning:\n%s", output)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(rawLog)
	for _, want := range []string{
		"container run --detach --name env-postgres",
		"--publish 127.0.0.1:55432:5432",
		"--volume env-volume:/var/lib/postgresql/data",
		"container exec env-postgres pg_isready -U envuser -d envdb",
		"OPENBRAIN_DATABASE_URL=postgres://envuser:envpass@127.0.0.1:55432/envdb?sslmode=disable",
		"OPENBRAIN_OLLAMA_URL=http://127.0.0.1:21434",
		"OPENBRAIN_CSRF_KEY=env-csrf-key",
		"curl -fsS http://127.0.0.1:21434/api/version",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("startup log missing %q:\n%s", want, logText)
		}
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
		"set -x",
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
