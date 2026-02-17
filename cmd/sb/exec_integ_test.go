//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Long tests execute commands under sandbox-exec and are skipped by `go test -short`.
func TestExecLong_PythonExample(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	copyFile(t, filepath.Join(repoRoot(t), "sandbox-exec-fun", "example.py"), filepath.Join(baseDir, "example.py"))

	python := findPython(t)
	code, out, err := runSandboxExecTest(baseDir, []string{
		python,
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

func TestExecLong_LsBlocked(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_ReadUsersBlocked(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_ReadCurrentDir(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_WriteViaSymlinkedCwd(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	linkRoot := userTempDir(t)
	linkPath := filepath.Join(linkRoot, "cwd-link")
	if err := os.Symlink(baseDir, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	target := filepath.Join(linkPath, "note.txt")
	code, _, err := runSandboxExecTest(linkPath, []string{"/bin/sh", "-c", "echo ok > note.txt"}, nil)
	if err != nil || code != 0 {
		t.Fatalf("write via symlink failed: code=%d err=%v", code, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("expected file not created: %v", err)
	}
}

func TestExecLong_PolicySensitivitySymlinkWriteDenied(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	linkRoot := userTempDir(t)
	linkPath := filepath.Join(linkRoot, "cwd-link")
	if err := os.Symlink(baseDir, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	profile := buildProfileForTest(t, linkPath, nil)
	profile = removeWriteRule(profile, filepath.Clean(baseDir))
	code, _, err := runSandboxExecWithProfile(linkPath, profile, []string{"/bin/sh", "-c", "echo ok > note.txt"}, nil)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("expected symlink write to fail without baseDirReal write allow")
	}
}

func TestExecLong_NetworkBlocked(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_PathAllowlistUnderUsers(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_NonPathUsersBlocked(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_GitOutsideBlocked(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_PolicySensitivityLsParent(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_PolicySensitivityUsersRead(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_PolicySensitivityNetwork(t *testing.T) {
	requireLongTest(t)
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

func TestExecLong_MinimalFSOptionRunsCommand(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/usr/bin/true"}, nil, sandboxProfileOptions{MinimalFS: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("minimal-fs mode command failed: code=%d err=%v out=%s", code, err, stdout.String()+stderr.String())
	}
}

func TestExecLong_MinimalFSCannotReadPersonalParentFile(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	parentDir := userTempDir(t)
	secretFile := filepath.Join(parentDir, "personal.txt")
	if err := os.WriteFile(secretFile, []byte("private"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/bin/cat", secretFile}, nil, sandboxProfileOptions{MinimalFS: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("cat failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("personal file outside cwd unexpectedly readable in minimal-fs mode: %s", stdout.String()+stderr.String())
	}
}

// --no-user tests

func TestExecLong_NoUserRunsCommand(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	var stdout, stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/usr/bin/true"}, nil, sandboxProfileOptions{NoUser: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("--no-user command failed: code=%d err=%v out=%s", code, err, stdout.String()+stderr.String())
	}
}

func TestExecLong_NoUserBlocksReadingUsersDir(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	secretFile := filepath.Join(userHomeForTests(t), ".CFUserTextEncoding")
	var stdout, stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/bin/cat", secretFile}, nil, sandboxProfileOptions{NoUser: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("cat failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("--no-user: reading file under /Users unexpectedly succeeded")
	}
}

func TestExecLong_NoUserBlocksAPFSFirmlinkToUsers(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	// /System/Volumes/Data/Users is the APFS firmlink that provides an
	// alternative path to /Users. Without an explicit deny, the broad
	// (allow file-read* (subpath "/System")) would grant access.
	home := userHomeForTests(t)
	username := strings.TrimPrefix(home, "/Users/")
	firmlinkPath := filepath.Join("/System/Volumes/Data/Users", username, ".CFUserTextEncoding")
	var stdout, stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/bin/cat", firmlinkPath}, nil, sandboxProfileOptions{NoUser: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("cat failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("--no-user: reading via APFS firmlink /System/Volumes/Data/Users unexpectedly succeeded")
	}
}

func TestExecLong_NoUserBlocksReadingCwd(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	testFile := filepath.Join(baseDir, "note.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/bin/cat", testFile}, nil, sandboxProfileOptions{NoUser: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("cat failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("--no-user: reading cwd file unexpectedly succeeded (cwd is under /Users)")
	}
}

func TestExecLong_NoUserPythonStdinStdout(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	python := findPython(t)
	baseDir := userTempDir(t)
	var stdout, stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(
		baseDir,
		[]string{python, "-IS", "-c", `import json,sys; data=json.load(sys.stdin); print(json.dumps({"n":len(data)}))`},
		map[string]string{"PYTHONDONTWRITEBYTECODE": "1"},
		sandboxProfileOptions{NoUser: true},
		strings.NewReader(`[1,2,3]`),
		&stdout, &stderr,
	)
	if err != nil || code != 0 {
		t.Fatalf("--no-user python stdin/stdout failed: code=%d err=%v out=%s", code, err, stdout.String()+stderr.String())
	}
	if !strings.Contains(stdout.String(), `"n": 3`) && !strings.Contains(stdout.String(), `"n":3`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestExecLong_NoUserBlocksNetwork(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	cmd := `python3 -c "import socket; socket.create_connection(('127.0.0.1', 80), timeout=1)"`
	var stdout, stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/bin/sh", "-c", cmd}, nil, sandboxProfileOptions{NoUser: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil {
		t.Fatalf("network test failed: %v", err)
	}
	if code == 0 {
		t.Fatalf("--no-user: network unexpectedly succeeded")
	}
}

func TestExecLong_NoUserWithRuntimeRunsCommand(t *testing.T) {
	requireLongTest(t)
	t.Parallel()
	baseDir := userTempDir(t)
	var stdout, stderr bytes.Buffer
	code, err := runSandboxExecWithOptions(baseDir, []string{"/usr/bin/true"}, nil, sandboxProfileOptions{NoUser: true, MinimalFS: true}, bytes.NewReader(nil), &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("--runtime --no-user command failed: code=%d err=%v out=%s", code, err, stdout.String()+stderr.String())
	}
}
