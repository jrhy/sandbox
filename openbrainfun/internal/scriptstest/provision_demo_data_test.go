package scriptstest

import (
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

func TestWalkthroughScriptUsesOpenbrainCLIForProvisioningAndStart(t *testing.T) {
	repo := repoRoot(t)
	script, err := os.ReadFile(filepath.Join(repo, "scripts", "walkthrough.sh"))
	if err != nil {
		t.Fatalf("read walkthrough.sh: %v", err)
	}
	text := string(script)
	if strings.Contains(text, "provision-demo-data.sh") {
		t.Fatalf("walkthrough should no longer call provision-demo-data.sh:\n%s", text)
	}
	if !strings.Contains(text, "go run ./cmd/openbrain user update") {
		t.Fatalf("walkthrough should provision through openbrain user update:\n%s", text)
	}
	if !strings.Contains(text, "\"$app_bin\" start") {
		t.Fatalf("walkthrough should start the server through the openbrain start subcommand:\n%s", text)
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
