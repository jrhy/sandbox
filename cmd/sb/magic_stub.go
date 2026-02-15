//go:build !libmagic

package main

import (
	"fmt"
	"strings"

	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["magic"] = subcommand{
		`[files...]`,
		"analyzes files (or stdin) with libmagic and outputs MIME type",
		func(a []string) int {
			o := struct{}{}
			p := flags.NewParser(&o, 0)
			_, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			fmt.Println("magic support is disabled; build with -tags libmagic")
			return 1
		},
	}
}
