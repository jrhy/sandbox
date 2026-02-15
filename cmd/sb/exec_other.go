//go:build !darwin

package main

import (
	"fmt"
	"os"
)

func init() {
	funcs["exec"] = subcommand{
		"<command> [args...]",
		"Run a command under a macOS sandbox profile",
		func(a []string) int {
			if len(a) == 0 {
				return exitSubcommandUsage
			}
			fmt.Fprintln(os.Stderr, "sb exec is TODO on this platform (possible future: bwrap/firejail)")
			return exitError
		},
	}
}
