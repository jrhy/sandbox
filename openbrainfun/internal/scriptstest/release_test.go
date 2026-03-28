package scriptstest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func copyTree(t *testing.T, srcDir, dstDir string) {
	t.Helper()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("read dir %s: %v", srcDir, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dstDir, err)
	}
	for _, entry := range entries {
		src := filepath.Join(srcDir, entry.Name())
		dst := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			copyTree(t, src, dst)
			continue
		}
		copyFile(t, src, dst)
	}
}

func makeReleaseFixture(t *testing.T, includeScript bool) string {
	t.Helper()
	repo := repoRoot(t)
	fixture := t.TempDir()
	for _, rel := range []string{
		".env.example",
		"scripts/init-env.sh",
		"scripts/startup-macos.sh",
		"docs/operations.md",
		"cmd/openbrain/main.go",
	} {
		copyFile(t, filepath.Join(repo, rel), filepath.Join(fixture, rel))
	}
	copyTree(t, filepath.Join(repo, "migrations"), filepath.Join(fixture, "migrations"))
	if includeScript {
		copyFile(t, filepath.Join(repo, "scripts", "release.sh"), filepath.Join(fixture, "scripts", "release.sh"))
	}
	return fixture
}

func writeFakeGo(t *testing.T, binDir, logPath string) {
	t.Helper()
	writeExecutable(t, filepath.Join(binDir, "go"), fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
printf 'pwd %%s\n' "$PWD" >> %q
printf 'go %%s\n' "$*" >> %q
if [[ "${1:-}" == "env" && "${2:-}" == "GOOS" ]]; then
  printf 'darwin\n'
  exit 0
fi
if [[ "${1:-}" == "env" && "${2:-}" == "GOARCH" ]]; then
  printf 'arm64\n'
  exit 0
fi
out=""
args=("$@")
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -z "$out" ]]; then
  echo 'missing -o output path' >&2
  exit 1
fi
mkdir -p "$(dirname "$out")"
printf '#!/usr/bin/env bash\necho fake openbrain\n' > "$out"
chmod 755 "$out"
if [[ "${args[*]}" != *"./cmd/openbrain"* ]]; then
  echo 'missing ./cmd/openbrain build target' >&2
  exit 1
fi
`, logPath, logPath))
}

func writeFakeGit(t *testing.T, binDir, logPath, revParseOutput, statusOutput string) {
	t.Helper()
	writeExecutable(t, filepath.Join(binDir, "git"), fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
printf 'pwd %%s\n' "$PWD" >> %q
printf 'git %%s\n' "$*" >> %q
if [[ "${1:-}" == "rev-parse" && "${2:-}" == "HEAD" ]]; then
  printf %q
  exit 0
fi
if [[ "${1:-}" == "status" && "${2:-}" == "--short" ]]; then
  printf %q
  exit 0
fi
echo "unexpected git invocation: $*" >&2
exit 1
`, logPath, logPath, revParseOutput, statusOutput))
}

func runReleaseScript(t *testing.T, fixture, workingDir string, pathDirs ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(fixture, "scripts", "release.sh"))
	cmd.Dir = workingDir
	pathValue := strings.Join(pathDirs, ":")
	if pathValue == "" {
		pathValue = "/usr/bin:/bin"
	}
	cmd.Env = append(os.Environ(), "PATH="+pathValue)
	return cmd.CombinedOutput()
}

func releaseBundlePaths(t *testing.T, fixture string) []string {
	t.Helper()
	paths := []string{
		"release/openbrain",
		"release/.env.example",
		"release/scripts/startup-macos.sh",
		"release/scripts/init-env.sh",
		"release/docs/operations.md",
		"release/RELEASE_MANIFEST.txt",
	}

	migrationEntries, err := os.ReadDir(filepath.Join(fixture, "migrations"))
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, entry := range migrationEntries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join("release", "migrations", entry.Name()))
	}
	slices.Sort(paths)
	return paths
}

func assertReleaseBundleExists(t *testing.T, fixture string) {
	t.Helper()
	for _, rel := range releaseBundlePaths(t, fixture) {
		assertExists(t, filepath.Join(fixture, rel))
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to not exist, got err=%v", path, err)
	}
}

func TestReleaseScriptBuildsCuratedBundle(t *testing.T) {
	fixture := makeReleaseFixture(t, true)
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	goLog := filepath.Join(t.TempDir(), "go.log")
	gitLog := filepath.Join(t.TempDir(), "git.log")
	writeFakeGo(t, binDir, goLog)
	writeFakeGit(t, binDir, gitLog, "abc123\n", "")

	output, err := runReleaseScript(t, fixture, fixture, binDir, "/usr/bin", "/bin")
	if err != nil {
		t.Fatalf("release.sh failed: %v\n%s", err, output)
	}

	assertReleaseBundleExists(t, fixture)

	if !strings.Contains(string(output), filepath.Join(fixture, "release")) {
		t.Fatalf("expected success summary to mention release path, got:\n%s", output)
	}

	rawGoLog, err := os.ReadFile(goLog)
	if err != nil {
		t.Fatalf("read go log: %v", err)
	}
	goLogText := string(rawGoLog)
	if !strings.Contains(goLogText, "go build -o ") || !strings.Contains(goLogText, " ./cmd/openbrain") {
		t.Fatalf("expected go build target in log, got:\n%s", rawGoLog)
	}
}

func TestReleaseScriptExcludesUserManagedStateAndRebuildsCleanly(t *testing.T) {
	fixture := makeReleaseFixture(t, true)
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	goLog := filepath.Join(t.TempDir(), "go.log")
	gitLog := filepath.Join(t.TempDir(), "git.log")
	writeFakeGo(t, binDir, goLog)
	writeFakeGit(t, binDir, gitLog, "abc123\n", "")

	for path, data := range map[string]string{
		".env":                           "OPENBRAIN_CSRF_KEY=secret\n",
		"var/postgres/test.txt":          "state\n",
		"docs/walkthrough.demo.md":       "demo\n",
		"docs/superpowers/specs/test.md": "spec\n",
		"docs/superpowers/plans/test.md": "plan\n",
		"internal/secret.txt":            "internal\n",
		"scripts/verify.sh":              "#!/usr/bin/env bash\n",
	} {
		full := filepath.Join(fixture, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	if output, err := runReleaseScript(t, fixture, fixture, binDir, "/usr/bin", "/bin"); err != nil {
		t.Fatalf("first release.sh run failed: %v\n%s", err, output)
	}
	stalePath := filepath.Join(fixture, "release", "stale.txt")
	if err := os.WriteFile(stalePath, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if output, err := runReleaseScript(t, fixture, fixture, binDir, "/usr/bin", "/bin"); err != nil {
		t.Fatalf("second release.sh run failed: %v\n%s", err, output)
	}

	assertNotExists(t, stalePath)
	for _, rel := range []string{
		"release/.env",
		"release/var",
		"release/docs/walkthrough.demo.md",
		"release/docs/superpowers/specs",
		"release/docs/superpowers/plans",
		"release/internal",
		"release/scripts/verify.sh",
	} {
		assertNotExists(t, filepath.Join(fixture, rel))
	}
	assertReleaseBundleExists(t, fixture)
}

func TestReleaseScriptWritesManifestMetadata(t *testing.T) {
	fixture := makeReleaseFixture(t, true)
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	writeFakeGo(t, binDir, filepath.Join(t.TempDir(), "go.log"))
	writeFakeGit(t, binDir, filepath.Join(t.TempDir(), "git.log"), "abc123\n", "")

	output, err := runReleaseScript(t, fixture, fixture, binDir, "/usr/bin", "/bin")
	if err != nil {
		t.Fatalf("release.sh failed: %v\n%s", err, output)
	}

	manifest, err := os.ReadFile(filepath.Join(fixture, "release", "RELEASE_MANIFEST.txt"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for _, want := range []string{"Commit: abc123", "Tree: clean", "Built:", "GOOS: darwin", "GOARCH: arm64"} {
		if !strings.Contains(string(manifest), want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
}

func TestReleaseScriptReportsDirtyTreeInManifest(t *testing.T) {
	fixture := makeReleaseFixture(t, true)
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	writeFakeGo(t, binDir, filepath.Join(t.TempDir(), "go.log"))
	writeFakeGit(t, binDir, filepath.Join(t.TempDir(), "git.log"), "abc123\n", " M scripts/release.sh\n")

	output, err := runReleaseScript(t, fixture, fixture, binDir, "/usr/bin", "/bin")
	if err != nil {
		t.Fatalf("release.sh failed: %v\n%s", err, output)
	}

	manifest, err := os.ReadFile(filepath.Join(fixture, "release", "RELEASE_MANIFEST.txt"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !strings.Contains(string(manifest), "Tree: dirty") {
		t.Fatalf("manifest missing dirty tree state:\n%s", manifest)
	}
}

func TestReleaseScriptFailsWithoutRequiredCommands(t *testing.T) {
	fixture := makeReleaseFixture(t, true)

	output, err := runReleaseScript(t, fixture, fixture, "/bin")
	if err == nil {
		t.Fatalf("release.sh unexpectedly succeeded without go:\n%s", output)
	}
	if !strings.Contains(string(output), "required command not found in PATH: go") {
		t.Fatalf("unexpected missing-go output:\n%s", output)
	}

	goOnlyDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(goOnlyDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	writeFakeGo(t, goOnlyDir, filepath.Join(t.TempDir(), "go.log"))
	output, err = runReleaseScript(t, fixture, fixture, goOnlyDir, "/bin")
	if err == nil {
		t.Fatalf("release.sh unexpectedly succeeded without git:\n%s", output)
	}
	if !strings.Contains(string(output), "required command not found in PATH: git") {
		t.Fatalf("unexpected missing-git output:\n%s", output)
	}
}

func TestReleaseScriptFailsWhenRequiredArtifactsAreMissing(t *testing.T) {
	fixture := makeReleaseFixture(t, true)
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	writeFakeGo(t, binDir, filepath.Join(t.TempDir(), "go.log"))
	writeFakeGit(t, binDir, filepath.Join(t.TempDir(), "git.log"), "abc123\n", "")

	missingPath := filepath.Join(fixture, "docs", "operations.md")
	if err := os.Remove(missingPath); err != nil {
		t.Fatalf("remove %s: %v", missingPath, err)
	}

	output, err := runReleaseScript(t, fixture, fixture, binDir, "/usr/bin", "/bin")
	if err == nil {
		t.Fatalf("release.sh unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), missingPath) {
		t.Fatalf("expected missing artifact path in output, got:\n%s", output)
	}
}

func TestReleaseScriptUsesRepoRootForGitAndGoMetadataWhenInvokedOutsideRepo(t *testing.T) {
	fixture := makeReleaseFixture(t, true)
	outsideDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	goLog := filepath.Join(t.TempDir(), "go.log")
	gitLog := filepath.Join(t.TempDir(), "git.log")
	writeFakeGo(t, binDir, goLog)
	writeFakeGit(t, binDir, gitLog, "abc123\n", "")

	output, err := runReleaseScript(t, fixture, outsideDir, binDir, "/usr/bin", "/bin")
	if err != nil {
		t.Fatalf("release.sh failed from outside repo: %v\n%s", err, output)
	}

	assertReleaseBundleExists(t, fixture)
	for _, logPath := range []string{goLog, gitLog} {
		rawLog, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read log %s: %v", logPath, err)
		}
		want := "pwd " + fixture
		if !strings.Contains(string(rawLog), want) {
			t.Fatalf("expected %s to run from repo root %q, got:\n%s", filepath.Base(logPath), want, rawLog)
		}
	}
}
