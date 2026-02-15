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

func TestExecPythonExample(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	copyFile(t, filepath.Join(repoRoot(t), "sandbox-exec-fun", "example.py"), filepath.Join(baseDir, "example.py"))

	code, out, err := runSandboxExecTest(baseDir, []string{
		"/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9/bin/python3",
		"-S",
		filepath.Join(baseDir, "example.py"),
	}, map[string]string{
		"PYTHONDONTWRITEBYTECODE": "1",
		"PYTHONNOUSERSITE":        "1",
	})
	if err != nil || code != 0 {
		t.Fatalf("python example failed: code=%d err=%v out=%s", code, err, out)
	}
	if !strings.Contains(out, "expected failure") {
		t.Fatalf("python example output unexpected: %s", out)
	}
}

func TestExecLsBlocked(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	code, _, err := runSandboxExecTest(baseDir, []string{"/bin/ls", ".."}, nil)
	if err != nil {
		t.Fatalf("ls failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("ls .. unexpectedly succeeded")
	}
}

func TestExecReadUsersBlocked(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	code, _, err := runSandboxExecTest(baseDir, []string{"/bin/cat", filepath.Join(userHomeForTests(t), ".CFUserTextEncoding")}, nil)
	if err != nil {
		t.Fatalf("cat failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("read from /Users unexpectedly succeeded")
	}
}

func TestExecReadCurrentDir(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	content := "hello"
	testFile := filepath.Join(baseDir, "note.txt")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	code, out, err := runSandboxExecTest(baseDir, []string{"/bin/cat", testFile}, nil)
	if err != nil || code != 0 {
		t.Fatalf("cat failed: code=%d err=%v out=%s", code, err, out)
	}
	if strings.TrimSpace(out) != content {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecNetworkBlocked(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	cmd := "python3 -c \"import socket; socket.create_connection(('127.0.0.1', 80), timeout=1)\""
	code, _, err := runSandboxExecTest(baseDir, []string{"/bin/sh", "-c", cmd}, nil)
	if err != nil {
		t.Fatalf("network test failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("network unexpectedly succeeded")
	}
}

func TestExecPathAllowlistUnderUsers(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	pathRoot := userTempDir(t)
	binDir := filepath.Join(pathRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	prog := filepath.Join(binDir, "hello-path")
	if err := os.WriteFile(prog, []byte("#!/bin/sh\necho hello from PATH\n"), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}
	code, out, err := runSandboxExecTest(baseDir, []string{"hello-path"}, map[string]string{
		"PATH": fmt.Sprintf("%s:/usr/bin:/bin", binDir),
	})
	if err != nil || code != 0 {
		t.Fatalf("PATH allowlist failed: code=%d err=%v out=%s", code, err, out)
	}
	if strings.TrimSpace(out) != "hello from PATH" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecNonPathUsersBlocked(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	secretRoot := userTempDir(t)
	secretFile := filepath.Join(secretRoot, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	code, _, err := runSandboxExecTest(baseDir, []string{"/bin/cat", secretFile}, nil)
	if err != nil {
		t.Fatalf("cat failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("non-PATH /Users read unexpectedly succeeded")
	}
}

func TestExecGitOutsideBlocked(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	repoDir := userTempDir(t)
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := runGit(repoDir, "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hi"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := runGit(repoDir, "add", "README.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runGit(repoDir, "-c", "user.name=sandbox", "-c", "user.email=sandbox@example.invalid", "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	code, _, err := runSandboxExecTest(baseDir, []string{"/usr/bin/git", "-C", repoDir, "status", "-sb"}, nil)
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if code == 0 {
		t.Fatalf("git outside unexpectedly succeeded")
	}
}

func TestExecPolicySensitivityLsParent(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	parentDir := filepath.Dir(baseDir)
	profile := buildProfileForTest(t, baseDir, nil)
	profile = removeParentDenyRules(profile)
	profile = removeUsersDenyRules(profile)
	profile = addAllowReadSubpath(profile, parentDir)
	code, _, err := runSandboxExecWithProfile(baseDir, profile, []string{"/bin/ls", ".."}, nil)
	if err != nil || code != 0 {
		t.Fatalf("expected ls .. to succeed with parent denies removed: code=%d err=%v", code, err)
	}
}

func TestExecPolicySensitivityUsersRead(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	secretRoot := userTempDir(t)
	secretFile := filepath.Join(secretRoot, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	profile := buildProfileForTest(t, baseDir, nil)
	profile = removeUsersDenyRules(profile)
	profile = addAllowReadSubpath(profile, secretRoot)
	code, _, err := runSandboxExecWithProfile(baseDir, profile, []string{"/bin/cat", secretFile}, nil)
	if err != nil || code != 0 {
		t.Fatalf("expected /Users read to succeed without deny rules: code=%d err=%v", code, err)
	}
}

func TestExecPolicySensitivityNetwork(t *testing.T) {
	t.Parallel()
	baseDir := userTempDir(t)
	profile := buildProfileForTest(t, baseDir, nil)
	profile = addAllowNetwork(profile)
	cmd := "python3 -c \"import socket; s=socket.socket(); print('ok')\""
	code, out, err := runSandboxExecWithProfile(baseDir, profile, []string{"/bin/sh", "-c", cmd}, nil)
	if err != nil || code != 0 {
		t.Fatalf("expected network socket creation to succeed with allow rule: code=%d err=%v out=%s", code, err, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func runSandboxExecTest(baseDir string, args []string, env map[string]string) (int, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := runSandboxExec(baseDir, args, env, bytes.NewReader(nil), &stdout, &stderr)
	out := stdout.String() + stderr.String()
	return code, out, err
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
	profile, err := buildSandboxProfile(baseDir, baseDirReal, userHome, tmpDir, pathEnv)
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
