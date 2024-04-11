package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["now"] = subcommand{
		`[--rfc1123 | --rfc3339]`,
		"prints the current time in a given format",
		func(a []string) int {
			now := time.Now()
			now.Format(time.RFC1123)
			o := struct {
				Rfc1123 bool `long:"rfc1123"`
				Rfc3339 bool `long:"rfc3339"`
			}{}
			p := flags.NewParser(&o, 0)
			_, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			switch {
			case o.Rfc1123:
				fmt.Println(now.Format(time.RFC1123))
			case o.Rfc3339:
				fmt.Println(now.Format(time.RFC3339))
			default:
				fmt.Println(now.Format(time.RFC3339))
			}
			return 0
		},
	}
}
