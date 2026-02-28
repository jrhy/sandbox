//go:build darwin

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type sandboxProfileOptions struct {
	AllowNetwork       bool
	MinimalFS          bool
	NoUser             bool
	HTTPAllowHosts     []string
	LocalhostProxyPort int
}

func init() {
	funcs["exec"] = subcommand{
		`[--minimal-fs] [--network] [--no-user] [--http-allow host-glob[,host-glob...]] <command> [args...]
    --minimal-fs    restrict access to cwd plus temp dirs, with minimal system/runtime reads (tuned for Go)
    --network       allow network access
    --http-allow    allow HTTP(S) only through a localhost proxy, filtered by hostname glob
    --no-user       deny ALL access under /Users; no cwd access; PATH entries under /Users are removed`,
		"Run a command under a macOS sandbox profile",
		func(a []string) int {
			opts, cmdArgs, err := parseSandboxExecArgs(a)
			if err != nil {
				die(fmt.Sprintf("parse: %v", err))
			}
			if len(cmdArgs) == 0 {
				return exitSubcommandUsage
			}
			baseDir, err := os.Getwd()
			if err != nil {
				die(fmt.Sprintf("cwd: %v", err))
			}
			exitCode, err := runSandboxExecWithOptions(baseDir, cmdArgs, nil, opts, os.Stdin, os.Stdout, os.Stderr)
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

func parseSandboxExecArgs(args []string) (sandboxProfileOptions, []string, error) {
	opts := sandboxProfileOptions{}
	remaining := args
	for len(remaining) > 0 {
		a := remaining[0]
		if a == "-h" || a == "--help" {
			return opts, nil, nil
		}
		if a == "--" {
			remaining = remaining[1:]
			break
		}
		if !strings.HasPrefix(a, "--") || a == "-" {
			break
		}
		if strings.HasPrefix(a, "--http-allow=") {
			patterns, err := parseHTTPAllowPatterns(strings.TrimPrefix(a, "--http-allow="))
			if err != nil {
				return sandboxProfileOptions{}, nil, err
			}
			opts.HTTPAllowHosts = append(opts.HTTPAllowHosts, patterns...)
			remaining = remaining[1:]
			continue
		}
		switch a {
		case "--minimal-fs", "--runtime", "--allow-runtime":
			opts.MinimalFS = true
		case "--network":
			opts.AllowNetwork = true
		case "--no-user":
			opts.NoUser = true
		case "--http-allow":
			if len(remaining) < 2 {
				return sandboxProfileOptions{}, nil, errors.New("missing value for --http-allow")
			}
			patterns, err := parseHTTPAllowPatterns(remaining[1])
			if err != nil {
				return sandboxProfileOptions{}, nil, err
			}
			opts.HTTPAllowHosts = append(opts.HTTPAllowHosts, patterns...)
			remaining = remaining[1:]
		default:
			return sandboxProfileOptions{}, nil, fmt.Errorf("unknown option %q", a)
		}
		remaining = remaining[1:]
	}
	if opts.AllowNetwork && len(opts.HTTPAllowHosts) > 0 {
		return sandboxProfileOptions{}, nil, errors.New("--network cannot be combined with --http-allow")
	}
	return opts, remaining, nil
}

func runSandboxExec(baseDir string, args []string, envOverride map[string]string, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
	return runSandboxExecWithOptions(baseDir, args, envOverride, sandboxProfileOptions{}, stdin, stdout, stderr)
}

func runSandboxExecWithOptions(baseDir string, args []string, envOverride map[string]string, opts sandboxProfileOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) (int, error) {
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

	if len(opts.HTTPAllowHosts) > 0 {
		proxy, err := startHTTPAllowlistProxy(opts.HTTPAllowHosts)
		if err != nil {
			return exitError, fmt.Errorf("http proxy: %w", err)
		}
		defer proxy.Close()
		opts.LocalhostProxyPort = proxy.port
		envOverride = mergeProxyEnv(envOverride, proxy.url)
	}

	profile, err := buildSandboxProfileWithOptions(baseDir, baseDirReal, userHome, tmpDir, pathEnv, opts)
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
	return buildSandboxProfileWithOptions(baseDir, baseDirReal, userHome, tmpDir, pathEnv, sandboxProfileOptions{})
}

func buildSandboxProfileWithOptions(baseDir, baseDirReal, userHome, tmpDir, pathEnv string, opts sandboxProfileOptions) (string, error) {
	buf := &bytes.Buffer{}
	buf.WriteString("(version 1)\n")
	buf.WriteString("(deny default)\n\n")
	buf.WriteString("(allow process*)\n")
	buf.WriteString("(allow sysctl-read)\n")
	buf.WriteString("(allow mach-lookup)\n\n")

	if opts.AllowNetwork {
		buf.WriteString("(allow network*)\n")
	} else if opts.LocalhostProxyPort > 0 {
		buf.WriteString(fmt.Sprintf("(allow network-outbound (remote tcp %s))\n", quoteProfile("localhost:"+strconv.Itoa(opts.LocalhostProxyPort))))
	}
	buf.WriteString("\n")

	buf.WriteString("(allow file-read* (subpath \"/System\") (subpath \"/usr\") (subpath \"/Library\") (subpath \"/System/Volumes/Data/Library\") (subpath \"/private\") (subpath \"/etc\") (subpath \"/dev\") (subpath \"/bin\") (subpath \"/sbin\"))\n")
	buf.WriteString("(allow file-map-executable (subpath \"/System\") (subpath \"/usr\") (subpath \"/Library\") (subpath \"/System/Volumes/Data/Library\") (subpath \"/bin\") (subpath \"/sbin\"))\n")
	buf.WriteString("(allow file-read-metadata (subpath \"/System/Cryptexes/App\") (subpath \"/System/Cryptexes/OS\"))\n")
	buf.WriteString("(allow file-write* (literal \"/dev/null\") (literal \"/dev/tty\") (literal \"/dev/fd\"))\n")
	// Many programs probe "/" during startup; allowing literal "/" does not grant recursive access.
	buf.WriteString("(allow file-read-data (literal \"/\"))\n")
	buf.WriteString("(allow file-read-metadata (literal \"/\"))\n")
	buf.WriteString("(allow file-read-metadata (subpath \"/var\"))\n")
	buf.WriteString("(allow file-ioctl (subpath \"/dev\"))\n")

	if opts.NoUser {
		// --no-user: no cwd access, no /Users access. Only system paths,
		// package manager roots (discovered from PATH), and a temp dir.
		buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(tmpDir)))
		buf.WriteString(fmt.Sprintf("(allow file-write* (subpath %s))\n", quoteProfile(tmpDir)))
		buf.WriteString(buildNoUserPathRules(pathEnv))
		buf.WriteString(buildNoUserDenyRules())
	} else {
		pathRules := buildPathRules(userHome, pathEnv)
		parentRules := buildParentRules(baseDir)
		userDenyRules := buildUserDenyRules(baseDir, baseDirReal, userHome, pathEnv)

		buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(baseDir)))
		buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(baseDirReal)))
		buf.WriteString(fmt.Sprintf("(allow file-write* (subpath %s))\n", quoteProfile(baseDir)))
		buf.WriteString(fmt.Sprintf("(allow file-write* (subpath %s))\n", quoteProfile(baseDirReal)))
		buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(tmpDir)))
		buf.WriteString(fmt.Sprintf("(allow file-write* (subpath %s))\n", quoteProfile(tmpDir)))
		buf.WriteString(pathRules)
		buf.WriteString(parentRules)
		buf.WriteString(userDenyRules)
	}

	if opts.MinimalFS {
		// Extra compatibility allowances for programs that probe host runtime state.
		buf.WriteString("(allow file-read-data (literal \"/dev/autofs_nowait\"))\n")
		buf.WriteString("(allow file-read-data (literal \"/dev/dtracehelper\"))\n")
		buf.WriteString("(allow file-read-data (literal \"/Library/Preferences/Logging/com.apple.diagnosticd.filter.plist\"))\n")
		if userHome != "" && !opts.NoUser {
			buf.WriteString(fmt.Sprintf("(allow file-read-data (literal %s))\n", quoteProfile(filepath.Join(userHome, ".CFUserTextEncoding"))))
			if strings.HasPrefix(userHome, "/Users/") {
				rel := strings.TrimPrefix(userHome, "/Users/")
				buf.WriteString(fmt.Sprintf("(allow file-read-data (literal %s))\n", quoteProfile(filepath.Join("/System/Volumes/Data/Users", rel, ".CFUserTextEncoding"))))
			}
		}
		buf.WriteString("(allow ipc-posix-shm-read-data)\n")
	}

	return buf.String(), nil
}

// buildNoUserPathRules discovers package manager roots from PATH entries
// (e.g., /opt/homebrew/bin → /opt/homebrew) and grants read-only access.
// Entries under /Users are skipped entirely. Entries already covered by
// system paths (/usr, /bin, /sbin, /Library, /System) are skipped.
func buildNoUserPathRules(pathEnv string) string {
	roots := map[string]bool{}
	for _, p := range strings.Split(pathEnv, ":") {
		p = filepath.Clean(p)
		if p == "" || p == "." {
			continue
		}
		// Skip /Users paths — the whole point of --no-user.
		if strings.HasPrefix(p, "/Users/") || strings.HasPrefix(p, "/System/Volumes/Data/Users/") {
			continue
		}
		// Skip paths already covered by broad system allows.
		if isSystemPath(p) {
			continue
		}
		root := packageManagerRoot(p)
		if root != "" {
			roots[root] = true
		}
	}
	var buf bytes.Buffer
	for root := range roots {
		// Parent directory metadata needed for path resolution
		// (e.g., /opt must be traversable to reach /opt/homebrew).
		parent := filepath.Dir(root)
		if parent != "/" {
			buf.WriteString(fmt.Sprintf("(allow file-read-metadata (literal %s))\n", quoteProfile(parent)))
		}
		buf.WriteString(fmt.Sprintf("(allow file-read* (subpath %s))\n", quoteProfile(root)))
		buf.WriteString(fmt.Sprintf("(allow file-map-executable (subpath %s))\n", quoteProfile(root)))
	}
	return buf.String()
}

func isSystemPath(p string) bool {
	for _, dir := range []string{"/usr", "/bin", "/sbin", "/Library", "/System", "/etc", "/private", "/dev"} {
		if p == dir || strings.HasPrefix(p, dir+"/") {
			return true
		}
	}
	return false
}

// packageManagerRoot returns the top-level root for known package managers,
// or the path itself for unknown non-system paths.
func packageManagerRoot(path string) string {
	switch {
	case strings.HasPrefix(path, "/opt/homebrew"):
		return "/opt/homebrew"
	case strings.HasPrefix(path, "/nix/"):
		return "/nix"
	default:
		return path
	}
}

// buildNoUserDenyRules produces hard deny rules for all user data paths.
// Placed at the end of the profile to override broader allows (e.g., /System
// covers /System/Volumes/Data/Users via APFS firmlink).
func buildNoUserDenyRules() string {
	return "(deny file-read* (subpath \"/Users\"))\n" +
		"(deny file-write* (subpath \"/Users\"))\n" +
		"(deny file-map-executable (subpath \"/Users\"))\n" +
		"(deny file-read* (subpath \"/System/Volumes/Data/Users\"))\n" +
		"(deny file-write* (subpath \"/System/Volumes/Data/Users\"))\n" +
		"(deny file-map-executable (subpath \"/System/Volumes/Data/Users\"))\n"
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

type sandboxHTTPProxy struct {
	server *http.Server
	port   int
	url    string
}

type allowlistHTTPProxy struct {
	patterns  []string
	transport http.RoundTripper
}

func parseHTTPAllowPatterns(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	patterns := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		pattern := strings.ToLower(strings.TrimSpace(part))
		if pattern == "" {
			continue
		}
		if _, err := path.Match(pattern, "example.com"); err != nil {
			return nil, fmt.Errorf("invalid --http-allow pattern %q: %w", part, err)
		}
		if seen[pattern] {
			continue
		}
		seen[pattern] = true
		patterns = append(patterns, pattern)
	}
	if len(patterns) == 0 {
		return nil, errors.New("missing hostname glob for --http-allow")
	}
	return patterns, nil
}

func startHTTPAllowlistProxy(patterns []string) (*sandboxHTTPProxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, fmt.Errorf("unexpected listener address %T", listener.Addr())
	}
	handler := &allowlistHTTPProxy{
		patterns: patterns,
		transport: &http.Transport{
			Proxy: nil,
		},
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = listener.Close()
		}
	}()
	return &sandboxHTTPProxy{
		server: server,
		port:   addr.Port,
		url:    "http://127.0.0.1:" + strconv.Itoa(addr.Port),
	}, nil
}

func (p *sandboxHTTPProxy) Close() error {
	if p == nil || p.server == nil {
		return nil
	}
	return p.server.Close()
}

func mergeProxyEnv(base map[string]string, proxyURL string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	out["HTTP_PROXY"] = proxyURL
	out["HTTPS_PROXY"] = proxyURL
	out["http_proxy"] = proxyURL
	out["https_proxy"] = proxyURL
	out["NO_PROXY"] = ""
	out["no_proxy"] = ""
	return out
}

func (p *allowlistHTTPProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodConnect {
		p.handleConnect(w, req)
		return
	}
	targetHost := req.URL.Hostname()
	if targetHost == "" {
		targetHost = hostFromAddress(req.Host)
	}
	if !p.hostAllowed(targetHost) {
		http.Error(w, "blocked by --http-allow", http.StatusForbidden)
		return
	}

	upstream := req.Clone(req.Context())
	upstream.RequestURI = ""
	if upstream.URL.Scheme == "" {
		upstream.URL.Scheme = "http"
	}
	if upstream.URL.Host == "" {
		upstream.URL.Host = req.Host
	}
	stripHopByHopHeaders(upstream.Header)

	resp, err := p.transport.RoundTrip(upstream)
	if err != nil {
		http.Error(w, "proxy upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	stripHopByHopHeaders(resp.Header)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *allowlistHTTPProxy) handleConnect(w http.ResponseWriter, req *http.Request) {
	targetAddr := req.Host
	if _, _, err := net.SplitHostPort(targetAddr); err != nil {
		targetAddr = net.JoinHostPort(targetAddr, "443")
	}
	if !p.hostAllowed(hostFromAddress(targetAddr)) {
		http.Error(w, "blocked by --http-allow", http.StatusForbidden)
		return
	}

	upstream, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		http.Error(w, "proxy upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = upstream.Close()
		http.Error(w, "proxy does not support hijacking", http.StatusInternalServerError)
		return
	}
	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		_ = upstream.Close()
		http.Error(w, "proxy hijack failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := clientBuf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		_ = clientConn.Close()
		_ = upstream.Close()
		return
	}
	if err := clientBuf.Flush(); err != nil {
		_ = clientConn.Close()
		_ = upstream.Close()
		return
	}

	go proxyCopy(upstream, io.MultiReader(clientBuf.Reader, clientConn), clientConn)
	go proxyCopy(clientConn, upstream, upstream)
}

func proxyCopy(dst io.WriteCloser, src io.Reader, closeConn io.Closer) {
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
	_ = closeConn.Close()
}

func (p *allowlistHTTPProxy) hostAllowed(host string) bool {
	host = normalizeProxyHost(host)
	if host == "" {
		return false
	}
	for _, pattern := range p.patterns {
		matched, err := path.Match(pattern, host)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func normalizeProxyHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	return host
}

func hostFromAddress(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return normalizeProxyHost(host)
	}
	return normalizeProxyHost(addr)
}

func stripHopByHopHeaders(h http.Header) {
	for _, key := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Proxy-Connection",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		h.Del(key)
	}
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
