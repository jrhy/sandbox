//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExecPythonExample(t *testing.T) {
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

func TestExecLsBlocked(t *testing.T) {
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

func TestExecReadUsersBlocked(t *testing.T) {
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

func TestExecReadCurrentDir(t *testing.T) {
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

func TestExecWriteViaSymlinkedCwd(t *testing.T) {
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

func TestExecPolicySensitivitySymlinkWriteDenied(t *testing.T) {
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

func TestExecNetworkBlocked(t *testing.T) {
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

func TestExecPathAllowlistUnderUsers(t *testing.T) {
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

func TestExecNonPathUsersBlocked(t *testing.T) {
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

func TestExecGitOutsideBlocked(t *testing.T) {
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

func TestExecPolicySensitivityLsParent(t *testing.T) {
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

func TestExecPolicySensitivityUsersRead(t *testing.T) {
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

func TestExecPolicySensitivityNetwork(t *testing.T) {
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

func TestParseSandboxExecArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		args     []string
		wantOpts sandboxProfileOptions
		wantCmd  []string
		wantErr  string
	}{
		{
			name:     "minimal-fs and network",
			args:     []string{"--minimal-fs", "--network", "/bin/echo", "hi"},
			wantOpts: sandboxProfileOptions{AllowNetwork: true, MinimalFS: true},
			wantCmd:  []string{"/bin/echo", "hi"},
		},
		{
			name:     "allow-runtime alias",
			args:     []string{"--allow-runtime", "/bin/echo"},
			wantOpts: sandboxProfileOptions{MinimalFS: true},
			wantCmd:  []string{"/bin/echo"},
		},
		{
			name:    "help",
			args:    []string{"-h"},
			wantCmd: nil,
		},
		{
			name:    "unknown option",
			args:    []string{"--fs-here-only", "/bin/echo"},
			wantErr: "unknown option",
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotOpts, gotCmd, err := parseSandboxExecArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if gotOpts != tc.wantOpts {
				t.Fatalf("unexpected options: got=%#v want=%#v", gotOpts, tc.wantOpts)
			}
			if !reflect.DeepEqual(gotCmd, tc.wantCmd) {
				t.Fatalf("unexpected command args: got=%#v want=%#v", gotCmd, tc.wantCmd)
			}
		})
	}
}

func TestExecMinimalFSOptionRunsCommand(t *testing.T) {
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

func TestExecMinimalFSCannotReadPersonalParentFile(t *testing.T) {
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

func TestBuildSandboxProfileWithOptions(t *testing.T) {
	t.Parallel()
	minimalFSRules := []string{
		"(allow file-read-data (literal \"/dev/autofs_nowait\"))",
		"(allow file-read-data (literal \"/dev/dtracehelper\"))",
		"(allow file-read-data (literal \"/Library/Preferences/Logging/com.apple.diagnosticd.filter.plist\"))",
		"(allow ipc-posix-shm-read-data)",
		fmt.Sprintf("(allow file-read-data (literal %s))", quoteProfile("/Users/tester/.CFUserTextEncoding")),
		fmt.Sprintf("(allow file-read-data (literal %s))", quoteProfile("/System/Volumes/Data/Users/tester/.CFUserTextEncoding")),
	}
	testCases := []struct {
		name           string
		opts           sandboxProfileOptions
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:        "minimal-fs includes runtime probes",
			opts:        sandboxProfileOptions{MinimalFS: true},
			mustContain: minimalFSRules,
		},
		{
			name:           "minimal-fs disabled omits runtime probes",
			opts:           sandboxProfileOptions{},
			mustNotContain: minimalFSRules,
		},
		{
			name:        "network option includes network allow rule",
			opts:        sandboxProfileOptions{AllowNetwork: true},
			mustContain: []string{"(allow network*)"},
		},
		{
			name:           "network disabled omits network allow rule",
			opts:           sandboxProfileOptions{},
			mustNotContain: []string{"(allow network*)"},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			profile, err := buildSandboxProfileWithOptions(
				"/Users/tester/work/repo",
				"/Users/tester/work/repo",
				"/Users/tester",
				"/tmp/sb-test",
				"/usr/bin:/bin:/Users/tester/bin",
				tc.opts,
			)
			if err != nil {
				t.Fatalf("profile: %v", err)
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(profile, want) {
					t.Fatalf("expected profile to contain %q", want)
				}
			}
			for _, deny := range tc.mustNotContain {
				if strings.Contains(profile, deny) {
					t.Fatalf("did not expect profile to contain %q", deny)
				}
			}
		})
	}
}

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
