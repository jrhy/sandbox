//go:build !darwin

package main

import (
	"fmt"
	"os"
)

func init() {
	funcs["exec"] = subcommand{
		`[--minimal-fs] [--network] [--no-user] <command> [args...]
    --minimal-fs    restrict access to cwd plus temp dirs, with minimal system/runtime reads (tuned for Go)
    --network       allow network access
    --no-user       deny ALL access under /Users; no cwd access; PATH entries under /Users are removed`,
		"Run a command under a macOS sandbox profile",
		func(a []string) int {
			if len(a) == 0 || (len(a) == 1 && (a[0] == "-h" || a[0] == "--help")) {
				return exitSubcommandUsage
			}
			fmt.Fprintln(os.Stderr, "sb exec is TODO on this platform (possible future: bwrap/firejail)")
			return exitError
		},
	}
}
