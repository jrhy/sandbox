//go:build darwin

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	funcs["exec"] = subcommand{
		"<command> [args...]",
		"Run a command under a macOS sandbox profile",
		func(a []string) int {
			if len(a) == 0 {
				return exitSubcommandUsage
			}
			baseDir, err := os.Getwd()
			if err != nil {
				die(fmt.Sprintf("cwd: %v", err))
			}
			exitCode, err := runSandboxExec(baseDir, a, nil, os.Stdin, os.Stdout, os.Stderr)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				if exitCode == 0 {
					return exitError
				}
			}
			return exitCode
		},
	}
}

func runSandboxExec(baseDir string, args []string, envOverride map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		return exitSubcommandUsage, errors.New("missing command")
	}

	baseDir = filepath.Clean(baseDir)
	baseDirReal := baseDir
	if real, err := filepath.EvalSymlinks(baseDir); err == nil {
		baseDirReal = real
	}

	userHome := os.Getenv("HOME")
	if userHome == "" || !strings.HasPrefix(userHome, "/Users/") {
		if u, err := user.Current(); err == nil && u.Username != "" {
			userHome = "/Users/" + u.Username
		}
	}

	tmpBase := os.Getenv("TMPDIR")
	if tmpBase == "" {
		tmpBase = "/tmp"
	}

	tmpDir, err := os.MkdirTemp(tmpBase, "sandbox-tmp.")
	if err != nil {
		return exitError, fmt.Errorf("tmpdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	profileFile, err := os.CreateTemp(tmpDir, "sandbox.profile.")
	if err != nil {
		return exitError, fmt.Errorf("profile: %w", err)
	}
	profilePath := profileFile.Name()
	defer os.Remove(profilePath)

	homeDir, err := os.MkdirTemp(tmpDir, "sandbox-home.")
	if err != nil {
		return exitError, fmt.Errorf("home: %w", err)
	}

	pathEnv := os.Getenv("PATH")
	if envOverride != nil {
		if v, ok := envOverride["PATH"]; ok {
			pathEnv = v
		}
	}

	profile, err := buildSandboxProfile(baseDir, baseDirReal, userHome, tmpDir, pathEnv)
	if err != nil {
		return exitError, err
	}

	if _, err := profileFile.WriteString(profile); err != nil {
		return exitError, fmt.Errorf("profile write: %w", err)
	}
	if err := profileFile.Close(); err != nil {
		return exitError, fmt.Errorf("profile close: %w", err)
	}

	cmd := exec.Command("/usr/bin/sandbox-exec", append([]string{"-f", profilePath}, args...)...)
	cmd.Dir = baseDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = mergeEnv(os.Environ(), map[string]string{
		"HOME":   homeDir,
		"TMPDIR": tmpDir,
		"TMP":    tmpDir,
		"TEMP":   tmpDir,
	}, envOverride)

	err = cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ProcessState.ExitCode(), nil
	}
	return exitError, err
}

func buildSandboxProfile(baseDir, baseDirReal, userHome, tmpDir, pathEnv string) (string, error) {
	pathRules := buildPathRules(userHome, pathEnv)
	parentRules := buildParentRules(baseDir)
	userDenyRules := buildUserDenyRules(baseDir, baseDirReal, userHome, pathEnv)

	buf := &bytes.Buffer{}
	buf.WriteString("(version 1)\n")
	buf.WriteString("(deny default)\n\n")
	buf.WriteString("(allow process*)\n")
	buf.WriteString("(allow sysctl-read)\n")
	buf.WriteString("(allow mach-lookup)\n\n")

	buf.WriteString("(allow file-read* (subpath \"/System\") (subpath \"/usr\") (subpath \"/Library\") (subpath \"/System/Volumes/Data/Library\") (subpath \"/private\") (subpath \"/etc\") (subpath \"/dev\") (subpath \"/bin\") (subpath \"/sbin\"))\n")
	buf.WriteString("(allow file-map-executable (subpath \"/System\") (subpath \"/usr\") (subpath \"/Library\") (subpath \"/System/Volumes/Data/Library\") (subpath \"/bin\") (subpath \"/sbin\"))\n")
	buf.WriteString("(allow file-read-metadata (subpath \"/System/Cryptexes/App\") (subpath \"/System/Cryptexes/OS\"))\n")
	buf.WriteString("(allow file-write* (literal \"/dev/null\") (literal \"/dev/tty\") (literal \"/dev/fd\"))\n")
	buf.WriteString("(allow file-read-data (literal \"/\"))\n")
	buf.WriteString("(allow file-read-metadata (literal \"/\"))\n")
	buf.WriteString("(allow file-read-metadata (subpath \"/var\"))\n")
	buf.WriteString("(allow file-ioctl (subpath \"/dev\"))\n")

	buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(baseDir)))
	buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(baseDirReal)))
	buf.WriteString(fmt.Sprintf("(allow file-write* (subpath %s))\n", quoteProfile(baseDir)))
	buf.WriteString(fmt.Sprintf("(allow file-write* (subpath %s))\n", quoteProfile(baseDirReal)))
	buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(tmpDir)))
	buf.WriteString(fmt.Sprintf("(allow file-write* (subpath %s))\n", quoteProfile(tmpDir)))

	buf.WriteString(pathRules)
	buf.WriteString(parentRules)
	buf.WriteString(userDenyRules)

	return buf.String(), nil
}

func buildPathRules(userHome, pathEnv string) string {
	if userHome == "" {
		return ""
	}
	var buf bytes.Buffer
	seen := map[string]bool{}
	for _, p := range strings.Split(pathEnv, ":") {
		p = filepath.Clean(p)
		if p == "." || p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if p == userHome {
			continue
		}
		buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(p)))
		buf.WriteString(fmt.Sprintf("(allow file-map-executable (subpath %s))\n", quoteProfile(p)))
	}
	return buf.String()
}

func buildParentRules(baseDir string) string {
	var buf bytes.Buffer
	parent := filepath.Clean(baseDir)
	for parent != "/" {
		parent = filepath.Dir(parent)
		if parent == "/" {
			break
		}
		buf.WriteString(fmt.Sprintf("(allow file-read-metadata (literal %s))\n", quoteProfile(parent)))
		buf.WriteString(fmt.Sprintf("(deny file-read-data (literal %s))\n", quoteProfile(parent)))
	}
	return buf.String()
}

func buildUserDenyRules(baseDir, baseDirReal, userHome, pathEnv string) string {
	prefixes := []string{}
	addPrefix := func(p string) {
		p = filepath.Clean(p)
		if !strings.HasPrefix(p, "/Users/") {
			return
		}
		if userHome != "" && p == userHome {
			return
		}
		rel := strings.TrimPrefix(p, "/Users/")
		if rel == "" {
			return
		}
		prefixes = append(prefixes, regexp.QuoteMeta(rel))
	}

	addPrefix(baseDir)
	addPrefix(baseDirReal)
	for _, p := range strings.Split(pathEnv, ":") {
		addPrefix(p)
	}

	if len(prefixes) == 0 {
		prefixes = []string{"__none__"}
	}
	allowGroup := strings.Join(prefixes, "|")

	return fmt.Sprintf("(allow file-read-metadata (subpath \"/Users\"))\n"+
		"(allow file-read-metadata (subpath \"/System/Volumes/Data/Users\"))\n"+
		"(deny file-read-data (regex #\"^/Users/(?!(%s)(/|$)).*\"))\n"+
		"(deny file-read-data (regex #\"^/System/Volumes/Data/Users/(?!(%s)(/|$)).*\"))\n"+
		"(deny file-map-executable (regex #\"^/Users/(?!(%s)(/|$)).*\"))\n"+
		"(deny file-map-executable (regex #\"^/System/Volumes/Data/Users/(?!(%s)(/|$)).*\"))\n",
		allowGroup, allowGroup, allowGroup, allowGroup,
	)
}

func quoteProfile(p string) string {
	return "\"" + strings.ReplaceAll(p, "\"", "\\\"") + "\""
}

func mergeEnv(base []string, add ...map[string]string) []string {
	out := map[string]string{}
	for _, kv := range base {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	for _, m := range add {
		for k, v := range m {
			out[k] = v
		}
	}
	res := make([]string, 0, len(out))
	for k, v := range out {
		res = append(res, k+"="+v)
	}
	return res
}
