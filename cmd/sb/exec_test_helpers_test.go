//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireLongTest(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration-style sandbox test in -short mode")
	}
	home := userHomeForTests(t)
	if _, err := os.Stat(home); err != nil {
		t.Skipf("home %q not accessible for integration-style sandbox test: %v", home, err)
	}
}

func runSandboxExecTest(baseDir string, args []string, env map[string]string) (int, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := runSandboxExec(baseDir, args, env, bytes.NewReader(nil), &stdout, &stderr)
	out := stdout.String() + stderr.String()
	return code, out, err
}

func findPython(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"/usr/bin/python3",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	t.Skip("python3 not found")
	return ""
}

func runSandboxExecWithProfile(baseDir, profile string, args []string, env map[string]string) (int, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	tmpDir, err := os.MkdirTemp("", "sb-exec-profile.")
	if err != nil {
		return exitError, "", err
	}
	defer os.RemoveAll(tmpDir)

	profilePath := filepath.Join(tmpDir, "sandbox.profile")
	if err := os.WriteFile(profilePath, []byte(profile), 0644); err != nil {
		return exitError, "", err
	}
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0700); err != nil {
		return exitError, "", err
	}
	cmd := exec.Command("/usr/bin/sandbox-exec", append([]string{"-f", profilePath}, args...)...)
	cmd.Dir = baseDir
	cmd.Env = mergeEnv(os.Environ(), map[string]string{
		"HOME":   homeDir,
		"TMPDIR": tmpDir,
		"TMP":    tmpDir,
		"TEMP":   tmpDir,
	}, env)
	cmd.Stdin = bytes.NewReader(nil)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		return 0, stdout.String() + stderr.String(), nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ProcessState.ExitCode(), stdout.String() + stderr.String(), nil
	}
	return exitError, stdout.String() + stderr.String(), err
}

func buildProfileForTest(t *testing.T, baseDir string, env map[string]string) string {
	return buildProfileForTestWithOptions(t, baseDir, env, sandboxProfileOptions{})
}

func buildProfileForTestWithOptions(t *testing.T, baseDir string, env map[string]string, opts sandboxProfileOptions) string {
	t.Helper()
	baseDirReal := baseDir
	if real, err := filepath.EvalSymlinks(baseDir); err == nil {
		baseDirReal = real
	}
	userHome := userHomeForTests(t)
	tmpDir, err := os.MkdirTemp("", "sb-exec-profile.")
	if err != nil {
		t.Fatalf("tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	pathEnv := os.Getenv("PATH")
	if env != nil {
		if v, ok := env["PATH"]; ok {
			pathEnv = v
		}
	}
	profile, err := buildSandboxProfileWithOptions(baseDir, baseDirReal, userHome, tmpDir, pathEnv, opts)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	return profile
}

func removeParentDenyRules(profile string) string {
	lines := strings.Split(profile, "\n")
	out := lines[:0]
	for _, line := range lines {
		if strings.Contains(line, "(deny file-read-data (literal ") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func removeUsersDenyRules(profile string) string {
	lines := strings.Split(profile, "\n")
	out := lines[:0]
	for _, line := range lines {
		if strings.Contains(line, "deny file-read-data") && strings.Contains(line, "/Users") {
			continue
		}
		if strings.Contains(line, "deny file-map-executable") && strings.Contains(line, "/Users") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func addAllowNetwork(profile string) string {
	lines := strings.Split(profile, "\n")
	out := make([]string, 0, len(lines)+1)
	inserted := false
	for _, line := range lines {
		out = append(out, line)
		if !inserted && strings.Contains(line, "(allow mach-lookup)") {
			out = append(out, "(allow network*)")
			inserted = true
		}
	}
	if !inserted {
		out = append(out, "(allow network*)")
	}
	return strings.Join(out, "\n")
}

func addAllowReadSubpath(profile, path string) string {
	line := fmt.Sprintf("(allow file-read* (subpath %s))", quoteProfile(path))
	return strings.TrimSpace(profile) + "\n" + line + "\n"
}

func removeWriteRule(profile, path string) string {
	lines := strings.Split(profile, "\n")
	out := lines[:0]
	want := fmt.Sprintf("(allow file-write* (subpath %s))", quoteProfile(path))
	for _, line := range lines {
		if strings.TrimSpace(line) == want {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func userTempDir(t *testing.T) string {
	home := userHomeForTests(t)
	dir, err := os.MkdirTemp(home, "sb-exec-test.")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func userHomeForTests(t *testing.T) string {
	home := os.Getenv("HOME")
	if home == "" || !strings.HasPrefix(home, "/Users/") {
		home = "/Users/" + os.Getenv("USER")
	}
	if home == "/Users/" {
		t.Fatalf("unable to determine user home")
	}
	return home
}

func repoRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("repo root not found")
	return ""
}

func copyFile(t *testing.T, src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("/usr/bin/git", args...)
	cmd.Dir = dir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
