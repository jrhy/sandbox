package main

import (
	"bufio"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// readJiraCLIConfig extracts top-level scalar keys (server, login) from the
// jira-cli YAML config without a YAML dependency.
func readJiraCLIConfig() map[string]string {
	out := map[string]string{}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	path := os.Getenv("JIRA_CONFIG_FILE")
	if path == "" {
		path = filepath.Join(home, ".config", ".jira", ".config.yml")
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if v != "" {
			out[strings.TrimSpace(k)] = v
		}
	}
	return out
}

// tokenFromNetrc finds a password for the Jira host in ~/.netrc.
func tokenFromNetrc(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(home, ".netrc"))
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(b))
	match := false
	for i := 0; i+1 < len(fields); i++ {
		switch fields[i] {
		case "machine":
			match = fields[i+1] == u.Hostname()
		case "password":
			if match {
				return fields[i+1]
			}
		}
	}
	return ""
}

// tokenFromKeychain reads the token jira-cli stores in the macOS keychain
// (service "jira-cli", account = login). Empty on other platforms.
func tokenFromKeychain(login string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("security", "find-generic-password", "-s", "jira-cli", "-a", login, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
