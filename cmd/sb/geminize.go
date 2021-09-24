package main

import (
	"fmt"
	"strings"

	"github.com/jessevdk/go-flags"
	"github.com/jrhy/sandbox/geminize"
)

func init() {
	funcs["geminize"] = subcommand{
		`-l example.com:8444
    [--autocert-cache=<path> --autocert-name=example.com]
    [--tls-cert=tls.crt --tls-key=tls.key]`,
		"proxy server that strips out inaccessible/tracking content, like Project Gemini",
		func(a []string) int {
			o := geminize.Options{
				Address: "0:8091",
			}
			p := flags.NewParser(&o, 0)
			ra, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if o.Address == "" && o.File == "" || len(ra) > 0 {
				return exitSubcommandUsage
			}
			if err := geminize.Run(o); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}
