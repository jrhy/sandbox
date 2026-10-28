//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Short tests exercise parsing, profile construction, and local helpers without sandbox-exec.
func TestExecShort_ParseSandboxExecArgs(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		args     []string
		wantOpts sandboxProfileOptions
		wantCmd  []string
		wantErr  string
	}{
		{
			name:     "gpu",
			args:     []string{"--gpu", "/bin/echo", "hi"},
			wantOpts: sandboxProfileOptions{AllowGPU: true},
			wantCmd:  []string{"/bin/echo", "hi"},
		},
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
			name:     "http allow",
			args:     []string{"--http-allow", "*.example.com,api.test.local", "/bin/echo"},
			wantOpts: sandboxProfileOptions{HTTPAllowHosts: []string{"*.example.com", "api.test.local"}},
			wantCmd:  []string{"/bin/echo"},
		},
		{
			name:     "localhost allow",
			args:     []string{"--localhost-allow", "5432,6379", "/bin/echo"},
			wantOpts: sandboxProfileOptions{LocalhostAllowPorts: []int{5432, 6379}},
			wantCmd:  []string{"/bin/echo"},
		},
		{
			name:     "localhost listen allows repeated port lists and both forms",
			args:     []string{"--localhost-listen-allow", "5432,6379", "--localhost-listen-allow=8080", "/bin/echo"},
			wantOpts: sandboxProfileOptions{LocalhostListenPorts: []int{5432, 6379, 8080}},
			wantCmd:  []string{"/bin/echo"},
		},
		{
			name:    "localhost listen missing value",
			args:    []string{"--localhost-listen-allow"},
			wantErr: "missing value for --localhost-listen-allow",
		},
		{
			name:    "localhost listen invalid port",
			args:    []string{"--localhost-listen-allow=65536", "/bin/echo"},
			wantErr: "must be between 1 and 65535",
		},
		{
			name: "localhost allow with http allow",
			args: []string{"--http-allow", "*.example.com", "--localhost-allow", "5432", "/bin/echo"},
			wantOpts: sandboxProfileOptions{
				HTTPAllowHosts:      []string{"*.example.com"},
				LocalhostAllowPorts: []int{5432},
			},
			wantCmd: []string{"/bin/echo"},
		},
		{
			name:     "allow read and write, both forms, repeatable",
			args:     []string{"--allow-read", "/a", "--allow-read=/b", "--allow-write", "/c", "--allow-write=/d", "/bin/echo"},
			wantOpts: sandboxProfileOptions{AllowRead: []string{"/a", "/b"}, AllowWrite: []string{"/c", "/d"}},
			wantCmd:  []string{"/bin/echo"},
		},
		{
			name:     "home, both forms",
			args:     []string{"--home", "/h", "/bin/echo"},
			wantOpts: sandboxProfileOptions{HomeDir: "/h"},
			wantCmd:  []string{"/bin/echo"},
		},
		{
			name:    "home missing value",
			args:    []string{"--home=", "/bin/echo"},
			wantErr: "missing value for --home",
		},
		{
			name:    "allow read missing value",
			args:    []string{"--allow-read"},
			wantErr: "missing value for --allow-read",
		},
		{
			name:    "allow write empty value",
			args:    []string{"--allow-write=", "/bin/echo"},
			wantErr: "missing value for --allow-write",
		},
		{
			name:    "help",
			args:    []string{"-h"},
			wantCmd: nil,
		},
		{
			name:    "network and http allow conflict",
			args:    []string{"--network", "--http-allow", "*.example.com", "/bin/echo"},
			wantErr: "cannot be combined",
		},
		{
			name:    "network and localhost allow conflict",
			args:    []string{"--network", "--localhost-allow", "5432", "/bin/echo"},
			wantErr: "cannot be combined",
		},
		{
			name:    "network and localhost listen conflict",
			args:    []string{"--network", "--localhost-listen-allow", "5432", "/bin/echo"},
			wantErr: "cannot be combined",
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
			if !reflect.DeepEqual(gotOpts, tc.wantOpts) {
				t.Fatalf("unexpected options: got=%#v want=%#v", gotOpts, tc.wantOpts)
			}
			if !reflect.DeepEqual(gotCmd, tc.wantCmd) {
				t.Fatalf("unexpected command args: got=%#v want=%#v", gotCmd, tc.wantCmd)
			}
		})
	}
}

func TestExecShort_ParseNoUser(t *testing.T) {
	t.Parallel()
	opts, cmd, err := parseSandboxExecArgs([]string{"--no-user", "/bin/echo", "hi"})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !opts.NoUser {
		t.Fatalf("expected NoUser=true")
	}
	if !reflect.DeepEqual(cmd, []string{"/bin/echo", "hi"}) {
		t.Fatalf("unexpected cmd: %v", cmd)
	}
}

func TestExecShort_ParseNoUserWithRuntime(t *testing.T) {
	t.Parallel()
	opts, cmd, err := parseSandboxExecArgs([]string{"--runtime", "--no-user", "python3", "-c", "1"})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	want := sandboxProfileOptions{MinimalFS: true, NoUser: true}
	if !reflect.DeepEqual(opts, want) {
		t.Fatalf("unexpected options: got=%#v want=%#v", opts, want)
	}
	if !reflect.DeepEqual(cmd, []string{"python3", "-c", "1"}) {
		t.Fatalf("unexpected cmd: %v", cmd)
	}
}

// Short tests keep /Users behavior checks deterministic by asserting generated profile text directly.
func TestExecShort_BuildSandboxProfileWithOptions(t *testing.T) {
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
			name: "gpu grants only selected Metal user clients",
			opts: sandboxProfileOptions{AllowGPU: true},
			mustContain: []string{
				"(iokit-user-client-class \"AGXDeviceUserClient\")",
				"(iokit-user-client-class \"IOSurfaceRootUserClient\")",
			},
			mustNotContain: []string{"(allow iokit-open)", "(allow network*)"},
		},
		{
			name:           "gpu disabled by default",
			mustNotContain: []string{"iokit-open", "AGXDeviceUserClient", "IOSurfaceRootUserClient"},
		},
		{
			name: "gpu allows only Metal cache trees and their compiler extensions",
			opts: sandboxProfileOptions{AllowGPU: true, GPUCacheDir: "/private/var/folders/test/C"},
			mustContain: []string{
				`(allow file-read* file-write* (subpath "/private/var/folders/test/C/com.apple.metal"))`,
				`(allow file-read* file-write* (subpath "/private/var/folders/test/C/com.apple.metalfe"))`,
				`(allow file-read* file-write* (subpath "/private/var/folders/test/C/com.apple.gpuarchiver"))`,
				`(allow file-issue-extension (require-all (extension-class "com.apple.app-sandbox.read-write") (subpath "/private/var/folders/test/C/com.apple.metalfe")))`,
			},
			mustNotContain: []string{`(subpath "/private/var/folders/test/C")`, "(allow file-issue-extension)", "(allow network*)"},
		},
		{
			name:           "gpu cache grants require explicit GPU opt-in",
			opts:           sandboxProfileOptions{GPUCacheDir: "/private/var/folders/test/C"},
			mustNotContain: []string{"com.apple.metal", "com.apple.gpuarchiver", "file-issue-extension"},
		},
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
			name: "http allow option restricts network to localhost proxy",
			opts: sandboxProfileOptions{LocalhostProxyPort: 43123},
			mustContain: []string{
				"(allow network-outbound (remote tcp \"localhost:43123\"))",
			},
			mustNotContain: []string{"(allow network*)"},
		},
		{
			name: "localhost allow option restricts network to selected loopback ports",
			opts: sandboxProfileOptions{LocalhostAllowPorts: []int{43123}},
			mustContain: []string{
				"(allow network-outbound (remote tcp \"localhost:43123\"))",
			},
			mustNotContain: []string{"(allow network*)"},
		},
		{
			name: "localhost listen permits only selected inbound port",
			opts: sandboxProfileOptions{LocalhostListenPorts: []int{43123}},
			mustContain: []string{
				`(allow network-bind (local ip "localhost:43123"))`,
				`(allow network-inbound (local ip "localhost:43123"))`,
			},
			mustNotContain: []string{"(allow network*)", "(allow network-outbound"},
		},
		{
			name: "localhost allow composes with http allow proxy port",
			opts: sandboxProfileOptions{LocalhostProxyPort: 49999, LocalhostAllowPorts: []int{43123}},
			mustContain: []string{
				"(allow network-outbound (remote tcp \"localhost:49999\"))",
				"(allow network-outbound (remote tcp \"localhost:43123\"))",
			},
			mustNotContain: []string{"(allow network*)"},
		},
		{
			name:           "network disabled omits network allow rule",
			opts:           sandboxProfileOptions{},
			mustNotContain: []string{"(allow network*)"},
		},
		{
			name: "no-user denies /Users and /System/Volumes/Data/Users",
			opts: sandboxProfileOptions{NoUser: true},
			mustContain: []string{
				"(deny file-read* (subpath \"/Users\"))",
				"(deny file-write* (subpath \"/Users\"))",
				"(deny file-map-executable (subpath \"/Users\"))",
				"(deny file-read* (subpath \"/System/Volumes/Data/Users\"))",
				"(deny file-write* (subpath \"/System/Volumes/Data/Users\"))",
				"(deny file-map-executable (subpath \"/System/Volumes/Data/Users\"))",
			},
		},
		{
			name: "no-user omits cwd access rules",
			opts: sandboxProfileOptions{NoUser: true},
			mustNotContain: []string{
				fmt.Sprintf("(allow file-read* (subpath %s))", quoteProfile("/Users/tester/work/repo")),
				fmt.Sprintf("(allow file-write* (subpath %s))", quoteProfile("/Users/tester/work/repo")),
			},
		},
		{
			name: "no-user omits /Users PATH entries",
			opts: sandboxProfileOptions{NoUser: true},
			mustNotContain: []string{
				fmt.Sprintf("(allow file-read* (subpath %s))", quoteProfile("/Users/tester/bin")),
			},
		},
		{
			name: "no-user with minimal-fs omits .CFUserTextEncoding",
			opts: sandboxProfileOptions{NoUser: true, MinimalFS: true},
			mustContain: []string{
				// MinimalFS rules that don't touch /Users should still be present.
				"(allow file-read-data (literal \"/dev/autofs_nowait\"))",
				"(allow ipc-posix-shm-read-data)",
			},
			mustNotContain: []string{
				".CFUserTextEncoding",
			},
		},
		{
			name: "no-user discovers package manager roots from PATH",
			opts: sandboxProfileOptions{NoUser: true},
			mustContain: []string{
				// /opt/homebrew/bin → /opt/homebrew root
				fmt.Sprintf("(allow file-read* (subpath %s))", quoteProfile("/opt/homebrew")),
				fmt.Sprintf("(allow file-map-executable (subpath %s))", quoteProfile("/opt/homebrew")),
				// /nix/store/abc123/bin → /nix root
				fmt.Sprintf("(allow file-read* (subpath %s))", quoteProfile("/nix")),
				fmt.Sprintf("(allow file-map-executable (subpath %s))", quoteProfile("/nix")),
				// Parent metadata for /opt (needed for realpath traversal)
				"(allow file-read-metadata (literal \"/opt\"))",
			},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pathEnv := "/usr/bin:/bin:/Users/tester/bin"
			if tc.opts.NoUser {
				// Include Homebrew path to test package manager root discovery.
				pathEnv = "/usr/bin:/bin:/Users/tester/bin:/opt/homebrew/bin:/nix/store/abc123/bin"
			}
			profile, err := buildSandboxProfileWithOptions(
				"/Users/tester/work/repo",
				"/Users/tester/work/repo",
				"/Users/tester",
				"/tmp/sb-test",
				pathEnv,
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

func TestExecShort_ParseHTTPAllowPatterns(t *testing.T) {
	t.Parallel()
	got, err := parseHTTPAllowPatterns(" *.Example.com,api.test.local,,*.Example.com ")
	if err != nil {
		t.Fatalf("parseHTTPAllowPatterns failed: %v", err)
	}
	want := []string{"*.example.com", "api.test.local"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected patterns: got=%v want=%v", got, want)
	}
}

func TestExecShort_ParseHTTPAllowPatternsRejectsInvalidGlob(t *testing.T) {
	t.Parallel()
	if _, err := parseHTTPAllowPatterns("["); err == nil {
		t.Fatalf("expected invalid glob error")
	}
}

func TestExecShort_ParseHTTPAllowPatternsRejectsEmpty(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", " , , "} {
		if _, err := parseHTTPAllowPatterns(raw); err == nil || !strings.Contains(err.Error(), "missing hostname glob") {
			t.Fatalf("raw=%q: expected missing hostname glob error, got %v", raw, err)
		}
	}
}

func TestExecShort_ParseLocalhostAllowPorts(t *testing.T) {
	t.Parallel()
	got, err := parseLocalhostAllowPorts(" 5432, 6379,5432 ")
	if err != nil {
		t.Fatalf("parseLocalhostAllowPorts failed: %v", err)
	}
	want := []int{5432, 6379}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ports: got=%v want=%v", got, want)
	}
}

func TestExecShort_ParseLocalhostAllowPortsRejectsInvalid(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", " , ", "abc", "0", "-1", "65536"} {
		if _, err := parseLocalhostAllowPorts(raw); err == nil {
			t.Fatalf("raw=%q: expected error", raw)
		}
	}
}

func TestExecShort_ParseSandboxExecArgsHTTPAllowMissingValue(t *testing.T) {
	t.Parallel()
	_, _, err := parseSandboxExecArgs([]string{"--http-allow"})
	if err == nil || !strings.Contains(err.Error(), "missing value for --http-allow") {
		t.Fatalf("expected missing value error, got %v", err)
	}
}

func TestExecShort_ParseSandboxExecArgsLocalhostAllowMissingValue(t *testing.T) {
	t.Parallel()
	_, _, err := parseSandboxExecArgs([]string{"--localhost-allow"})
	if err == nil || !strings.Contains(err.Error(), "missing value for --localhost-allow") {
		t.Fatalf("expected missing value error, got %v", err)
	}
}

func TestExecShort_MergeProxyEnv(t *testing.T) {
	t.Parallel()
	got := mergeProxyEnv(map[string]string{
		"HTTP_PROXY": "http://old:1",
		"NO_PROXY":   "localhost",
		"CUSTOM":     "keep",
	}, "http://127.0.0.1:43123", "")
	wantProxy := "http://127.0.0.1:43123"
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		if got[key] != wantProxy {
			t.Fatalf("%s=%q want %q", key, got[key], wantProxy)
		}
	}
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		if got[key] != "" {
			t.Fatalf("%s=%q want empty", key, got[key])
		}
	}
	if got["CUSTOM"] != "keep" {
		t.Fatalf("CUSTOM=%q want keep", got["CUSTOM"])
	}
}

func TestExecShort_MergeProxyEnvWithNoProxyBypass(t *testing.T) {
	t.Parallel()
	noProxy := "localhost,127.0.0.1,::1"
	got := mergeProxyEnv(nil, "http://127.0.0.1:43123", noProxy)
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		if got[key] != noProxy {
			t.Fatalf("%s=%q want %q", key, got[key], noProxy)
		}
	}
}

func TestExecShort_HTTPAllowProxyFiltersHosts(t *testing.T) {
	t.Parallel()
	var seenHost string
	proxy := &allowlistHTTPProxy{
		patterns: []string{"*.example.com"},
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenHost = req.URL.Host
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
			}, nil
		}),
	}

	allowedReq := httptest.NewRequest(http.MethodGet, "http://api.example.com/demo", nil)
	allowedRec := httptest.NewRecorder()
	proxy.ServeHTTP(allowedRec, allowedReq)
	if allowedRec.Code != http.StatusAccepted {
		t.Fatalf("allowed request code=%d body=%q", allowedRec.Code, allowedRec.Body.String())
	}
	if seenHost != "api.example.com" {
		t.Fatalf("unexpected upstream host: %q", seenHost)
	}

	blockedReq := httptest.NewRequest(http.MethodGet, "http://blocked.test/demo", nil)
	blockedRec := httptest.NewRecorder()
	proxy.ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusForbidden {
		t.Fatalf("blocked request code=%d body=%q", blockedRec.Code, blockedRec.Body.String())
	}
}

func TestExecShort_HTTPAllowProxyUsesReqHostWhenURLHostMissing(t *testing.T) {
	t.Parallel()
	var seenHost string
	var seenScheme string
	proxy := &allowlistHTTPProxy{
		patterns: []string{"api.example.com"},
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenHost = req.URL.Host
			seenScheme = req.URL.Scheme
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/demo", nil)
	req.Host = "api.example.com"
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d body=%q", rec.Code, rec.Body.String())
	}
	if seenHost != "api.example.com" {
		t.Fatalf("unexpected upstream host: %q", seenHost)
	}
	if seenScheme != "http" {
		t.Fatalf("unexpected upstream scheme: %q", seenScheme)
	}
}

func TestExecShort_HTTPAllowProxyReturnsBadGatewayOnTransportError(t *testing.T) {
	t.Parallel()
	proxy := &allowlistHTTPProxy{
		patterns: []string{"api.example.com"},
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("boom")
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/demo", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status: %d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "proxy upstream error: boom") {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestExecShort_HTTPAllowProxyStripsHopByHopHeaders(t *testing.T) {
	t.Parallel()
	var seenConnection string
	var seenProxyAuth string
	proxy := &allowlistHTTPProxy{
		patterns: []string{"api.example.com"},
		transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenConnection = req.Header.Get("Connection")
			seenProxyAuth = req.Header.Get("Proxy-Authorization")
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Header: http.Header{
					"Connection":        []string{"keep-alive"},
					"Transfer-Encoding": []string{"chunked"},
					"X-Upstream":        []string{"ok"},
				},
				Body: io.NopCloser(strings.NewReader("ok")),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/demo", nil)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Proxy-Authorization", "Basic abc")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d body=%q", rec.Code, rec.Body.String())
	}
	if seenConnection != "" || seenProxyAuth != "" {
		t.Fatalf("hop-by-hop headers leaked upstream: connection=%q proxy-auth=%q", seenConnection, seenProxyAuth)
	}
	if rec.Header().Get("Connection") != "" || rec.Header().Get("Transfer-Encoding") != "" {
		t.Fatalf("hop-by-hop headers leaked downstream: headers=%v", rec.Header())
	}
	if rec.Header().Get("X-Upstream") != "ok" {
		t.Fatalf("unexpected downstream headers: %v", rec.Header())
	}
}

func TestExecShort_HTTPAllowProxyNormalizesHosts(t *testing.T) {
	t.Parallel()
	proxy := &allowlistHTTPProxy{patterns: []string{"*.example.com", "api.test.local"}}
	testCases := []struct {
		host string
		want bool
	}{
		{host: "API.EXAMPLE.COM.", want: true},
		{host: "api.test.local", want: true},
		{host: "Api.Test.Local.", want: true},
		{host: "blocked.test.local", want: false},
	}
	for _, tc := range testCases {
		if got := proxy.hostAllowed(tc.host); got != tc.want {
			t.Fatalf("host=%q got=%v want=%v", tc.host, got, tc.want)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecShort_IsForbiddenProxyIP(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		ip   string
		want bool
	}{
		// Forbidden: loopback
		{"ipv4 loopback", "127.0.0.1", true},
		{"ipv4 loopback range", "127.8.8.8", true},
		{"ipv6 loopback", "::1", true},
		{"ipv4-mapped ipv6 loopback", "::ffff:127.0.0.1", true},
		// Forbidden: unspecified
		{"ipv4 unspecified", "0.0.0.0", true},
		{"ipv6 unspecified", "::", true},
		// Forbidden: link-local (incl. cloud metadata at 169.254.169.254)
		{"ipv4 link-local", "169.254.169.254", true},
		{"ipv6 link-local", "fe80::1", true},
		// Forbidden: multicast (not a valid TCP target)
		{"ipv4 multicast", "224.0.0.1", true},
		{"ipv6 multicast", "ff02::1", true},
		// Allowed: RFC1918 private and IPv6 ULA (subnet-reachable)
		{"rfc1918 10/8", "10.1.2.3", false},
		{"rfc1918 172.16/12 low", "172.16.0.1", false},
		{"rfc1918 172.16/12 high", "172.31.255.255", false},
		{"rfc1918 192.168/16", "192.168.1.1", false},
		{"ipv6 ula", "fd00::1", false},
		// Allowed: CGNAT 100.64/10 (e.g. Tailscale tailnet)
		{"cgnat low", "100.64.0.1", false},
		{"cgnat high", "100.127.255.255", false},
		// Allowed: public unicast
		{"public ipv4", "1.2.3.4", false},
		{"public ipv4 2", "8.8.8.8", false},
		{"public ipv4 outside 172.16/12", "172.32.0.1", false},
		{"public ipv4 outside cgnat", "100.128.0.1", false},
		{"public ipv6", "2606:4700::1111", false},
	}
	for _, tc := range testCases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("test bug: %q is not a valid IP", tc.ip)
		}
		if got := isForbiddenProxyIP(ip); got != tc.want {
			t.Errorf("isForbiddenProxyIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

// Regression test: the proxy must not tunnel to loopback services even when
// the requested hostname matches the allowlist and DNS (or a literal) points
// at 127.0.0.1.
func TestExecShort_HTTPAllowProxyBlocksLoopbackDial(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("local service reached"))
	}))
	defer ts.Close()

	proxy, err := startHTTPAllowlistProxy([]string{"*"})
	if err != nil {
		t.Fatalf("startHTTPAllowlistProxy: %v", err)
	}
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.url)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET via proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET to loopback via proxy: status=%d want 403, body=%q", resp.StatusCode, body)
	}
}

// Regression test: CONNECT tunneling must not reach loopback services via a
// hostname (e.g. rebindable DNS) that resolves to 127.0.0.1.
func TestExecShort_HTTPAllowProxyBlocksLoopbackConnect(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("local service reached"))
	}))
	defer ts.Close()

	proxy, err := startHTTPAllowlistProxy([]string{"*"})
	if err != nil {
		t.Fatalf("startHTTPAllowlistProxy: %v", err)
	}
	defer proxy.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(proxy.url, "http://"))
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()
	// Use the hostname "localhost" (not a literal) so the dial exercises DNS
	// resolution; localhost resolves to 127.0.0.1 and must still be blocked.
	port := ts.Listener.Addr().(*net.TCPAddr).Port
	target := net.JoinHostPort("localhost", strconv.Itoa(port))
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("CONNECT to loopback via proxy: status=%d want 403", resp.StatusCode)
	}
}

func TestExecShort_BuildAllowPathRules(t *testing.T) {
	t.Parallel()
	got := buildAllowPathRules([]string{"/Users/x/ro"}, []string{"/Users/x/rw"})
	for _, want := range []string{
		`(allow file-read* (subpath "/Users/x/ro"))`,
		`(allow file-read* (subpath "/Users/x/rw"))`,
		`(allow file-write* (subpath "/Users/x/rw"))`,
		`(allow file-read-metadata (literal "/Users/x"))`,
		`(allow file-read-metadata (literal "/Users"))`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing rule %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `(allow file-write* (subpath "/Users/x/ro"))`) {
		t.Errorf("read-only path got a write rule:\n%s", got)
	}
}

func TestExecShort_AllowPathRulesFollowUsersDeny(t *testing.T) {
	t.Parallel()
	profile, err := buildSandboxProfileWithOptions("/Users/x/cwd", "/Users/x/cwd", "/Users/x", "/tmp/t", "/usr/bin", sandboxProfileOptions{AllowRead: []string{"/Users/x/ro"}})
	if err != nil {
		t.Fatal(err)
	}
	deny := strings.Index(profile, "(deny file-read-data (require-all")
	allow := strings.Index(profile, `(allow file-read* (subpath "/Users/x/ro"))`)
	if deny < 0 || allow < 0 || allow < deny {
		t.Fatalf("allow rule must come after the /Users deny rules (deny=%d allow=%d):\n%s", deny, allow, profile)
	}
}

func TestExecShort_WriteMktempShim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	shimDir, err := writeMktempShim(dir)
	if err != nil {
		t.Fatalf("writeMktempShim: %v", err)
	}
	shimPath := filepath.Join(shimDir, "mktemp")
	info, err := os.Stat(shimPath)
	if err != nil {
		t.Fatalf("stat shim: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("shim not executable: %v", info.Mode())
	}
	b, err := os.ReadFile(shimPath)
	if err != nil {
		t.Fatalf("read shim: %v", err)
	}
	if !bytes.Contains(b, []byte("/usr/bin/mktemp")) {
		t.Fatalf("shim missing real mktemp path")
	}
	if !bytes.Contains(b, []byte("-p \"${TMPDIR:-/tmp}\"")) {
		t.Fatalf("shim missing -p injection")
	}
}

func TestExecShort_MktempShimArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		prefix string
		inTemp bool
		isDir  bool
	}{
		{name: "default", inTemp: true, prefix: "tmp."},
		{name: "directory", args: []string{"-d"}, inTemp: true, prefix: "tmp.", isDir: true},
		{name: "prefix", args: []string{"-t", "review"}, inTemp: true, prefix: "review."},
		{name: "grouped prefix", args: []string{"-dt", "review"}, inTemp: true, prefix: "review.", isDir: true},
		{name: "grouped quiet prefix", args: []string{"-qt", "review"}, inTemp: true, prefix: "review."},
		{name: "attached prefix", args: []string{"-dtreview"}, inTemp: true, prefix: "review.", isDir: true},
		{name: "long directory", args: []string{"--directory"}, inTemp: true, prefix: "tmp.", isDir: true},
		{name: "empty option terminator", args: []string{"--"}, inTemp: true, prefix: "tmp."},
		{name: "template", args: []string{"review.XXXXXX"}, prefix: "review."},
		{name: "template after terminator", args: []string{"--", "-review.XXXXXX"}, prefix: "-review."},
		{name: "grouped explicit directory", args: []string{"-qp", ".", "review.XXXXXX"}, prefix: "./review."},
		{name: "attached explicit directory", args: []string{"-p.", "review.XXXXXX"}, prefix: "./review."},
		{name: "long explicit directory", args: []string{"--tmpdir=.", "review.XXXXXX"}, prefix: "./review."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cwd := t.TempDir()
			tmpDir := t.TempDir()
			shimDir, err := writeMktempShim(tmpDir)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(filepath.Join(shimDir, "mktemp"), tt.args...)
			cmd.Dir = cwd
			cmd.Env = mergeEnv(os.Environ(), map[string]string{"TMPDIR": tmpDir})
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("mktemp %q: %v: %s", tt.args, err, out)
			}
			got := strings.TrimSpace(string(out))
			prefix := tt.prefix
			if tt.inTemp {
				prefix = filepath.Join(tmpDir, prefix)
			}
			if !strings.HasPrefix(got, prefix) {
				t.Fatalf("mktemp %q returned %q; want prefix %q", tt.args, got, prefix)
			}
			created := got
			if !filepath.IsAbs(created) {
				created = filepath.Join(cwd, created)
			}
			info, err := os.Stat(created)
			if err != nil {
				t.Fatal(err)
			}
			if info.IsDir() != tt.isDir {
				t.Fatalf("IsDir() = %v; want %v", info.IsDir(), tt.isDir)
			}
		})
	}
}

func TestExecShort_ResolveAllowPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveAllowPaths(dir, []string{"sub"})
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(filepath.Join(dir, "sub"))
	want := []string{filepath.Join(dir, "sub")}
	if real != want[0] {
		want = append(want, real)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := resolveAllowPaths(dir, []string{"missing"}); err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}
