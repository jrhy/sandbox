package main

import (
	"fmt"
	"strings"

	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["hello"] = subcommand{
		``,
		"minimal subcommand",
		func(a []string) int {
			o := struct{}{}
			p := flags.NewParser(&o, 0)
			ra, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if len(ra) > 0 {
				return exitSubcommandUsage
			}
			fmt.Printf("wazzzzzzzzzzzzzzzup\n")
			return 0
		},
	}
}
