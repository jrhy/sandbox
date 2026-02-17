//go:build darwin

package main

import (
	"fmt"
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
