package scriptstest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func TestProvisionDemoDataUsesWorkspaceLocalGoCachesWhenUnset(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	goEnvFile := filepath.Join(tempDir, "go-env.txt")
	writeExecutable(t, filepath.Join(binDir, "go"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'GOCACHE=%s\n' \"${GOCACHE-}\" > \"$GO_ENV_FILE\"\nprintf 'GOMODCACHE=%s\n' \"${GOMODCACHE-}\" >> \"$GO_ENV_FILE\"\nprintf 'GOWORK=%s\n' \"${GOWORK-}\" >> \"$GO_ENV_FILE\"\nprintf 'password_hash=fake-password-hash\n'\nprintf 'token_hash=fake-token-hash\n'\n")
	writeExecutable(t, filepath.Join(binDir, "podman-compose"), "#!/usr/bin/env bash\nset -euo pipefail\ncat >/dev/null\n")

	cmd := exec.Command("bash", "scripts/provision-demo-data.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin",
		"GO_ENV_FILE="+goEnvFile,
		"GOCACHE=",
		"GOMODCACHE=",
		"GOWORK=",
		"OPENBRAIN_DEMO_USERNAME=test-user",
		"OPENBRAIN_DEMO_PASSWORD=test-password",
		"OPENBRAIN_DEMO_MCP_TOKEN=test-token",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("provision-demo-data.sh failed: %v\n%s", err, output)
	}

	rawEnv, err := os.ReadFile(goEnvFile)
	if err != nil {
		t.Fatalf("read go env file: %v", err)
	}

	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(rawEnv)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected env line: %q", line)
		}
		values[parts[0]] = parts[1]
	}

	wantCache := filepath.Join(repo, ".cache", "go-build")
	wantModCache := filepath.Join(repo, ".cache", "gomod")
	if got := values["GOCACHE"]; got != wantCache {
		t.Fatalf("GOCACHE = %q, want %q\nscript output:\n%s", got, wantCache, bytes.TrimSpace(output))
	}
	if got := values["GOMODCACHE"]; got != wantModCache {
		t.Fatalf("GOMODCACHE = %q, want %q", got, wantModCache)
	}
	if got := values["GOWORK"]; got != "off" {
		t.Fatalf("GOWORK = %q, want %q", got, "off")
	}
}

func TestContainerComposePrefersPodmanCompose(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	logPath := filepath.Join(tempDir, "container-runtime.log")
	writeExecutable(t, filepath.Join(binDir, "podman-compose"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'podman-compose %s\\n' \"$*\" > \"$CONTAINER_RUNTIME_LOG\"\n")
	writeExecutable(t, filepath.Join(binDir, "podman"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'podman %s\\n' \"$*\" > \"$CONTAINER_RUNTIME_LOG\"\n")
	writeExecutable(t, filepath.Join(binDir, "docker"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'docker %s\\n' \"$*\" > \"$CONTAINER_RUNTIME_LOG\"\n")

	cmd := exec.Command("bash", "-c", ". scripts/container-runtime.sh && container_compose up -d postgres")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin",
		"CONTAINER_RUNTIME_LOG="+logPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("container_compose failed: %v\n%s", err, output)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got, want := strings.TrimSpace(string(rawLog)), "podman-compose up -d postgres"; got != want {
		t.Fatalf("selected runtime = %q, want %q", got, want)
	}
}

func TestProvisionDemoDataRunsRealGoHelperWithinModule(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	writeExecutable(t, filepath.Join(binDir, "podman-compose"), "#!/usr/bin/env bash\nset -euo pipefail\ncat >/dev/null\n")

	cmd := exec.Command("bash", "scripts/provision-demo-data.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"GOCACHE=",
		"GOMODCACHE=",
		"GOWORK=",
		"OPENBRAIN_DEMO_USERNAME=test-user",
		"OPENBRAIN_DEMO_PASSWORD=test-password",
		"OPENBRAIN_DEMO_MCP_TOKEN=test-token",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("provision-demo-data.sh failed with real go: %v\n%s", err, output)
	}
}

func TestRunRealEmbedTestsFallsBackToContainerizedOllama(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	logPath := filepath.Join(tempDir, "real-embed.log")
	writeExecutable(t, filepath.Join(binDir, "curl"), "#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n")
	writeExecutable(t, filepath.Join(binDir, "go"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'go %s\\n' \"$*\" >> \"$SCRIPT_LOG\"\n")
	writeExecutable(t, filepath.Join(binDir, "podman-compose"), "#!/usr/bin/env bash\nset -euo pipefail\nprintf 'podman-compose %s\\n' \"$*\" >> \"$SCRIPT_LOG\"\n")

	cmd := exec.Command("bash", ".github/scripts/run-real-embed-tests.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":/usr/bin:/bin",
		"SCRIPT_LOG="+logPath,
		"GOCACHE=",
		"GOMODCACHE=",
		"GOWORK=",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run-real-embed-tests.sh failed: %v\n%s", err, output)
	}

	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(rawLog)
	if !strings.Contains(logText, "podman-compose exec -T ollama ollama pull all-minilm:22m") {
		t.Fatalf("expected podman-compose Ollama pull in log, got:\n%s", logText)
	}
	if !strings.Contains(logText, "podman-compose exec -T ollama ollama pull qwen3:0.6b") {
		t.Fatalf("expected metadata model pull in log, got:\n%s", logText)
	}
}
