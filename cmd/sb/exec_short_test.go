//go:build darwin

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Short tests validate parsing/profile construction only and should be safe for `go test -short`.
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

func TestExecShort_ParseSandboxExecArgsHTTPAllowMissingValue(t *testing.T) {
	t.Parallel()
	_, _, err := parseSandboxExecArgs([]string{"--http-allow"})
	if err == nil || !strings.Contains(err.Error(), "missing value for --http-allow") {
		t.Fatalf("expected missing value error, got %v", err)
	}
}

func TestExecShort_MergeProxyEnv(t *testing.T) {
	t.Parallel()
	got := mergeProxyEnv(map[string]string{
		"HTTP_PROXY": "http://old:1",
		"NO_PROXY":   "localhost",
		"CUSTOM":     "keep",
	}, "http://127.0.0.1:43123")
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
