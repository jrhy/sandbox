package main

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/jessevdk/go-flags"
	"github.com/jrhy/sandbox/sql"
)

func init() {
	funcs["sql"] = subcommand{
		`[-f file] [-k] [SQL...]`,
		"pretty-print SQL expression WIP",
		func(a []string) int {
			o := struct {
				File     string `short:"f" long:"file" description:"Path to expression file"`
				Keywords string `short:"k" description:"yell keywords"`
			}{}
			p := flags.NewParser(&o, 0)
			ra, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			var query string
			if len(ra) > 0 && o.File != "" {
				die("both -f and arguments")
			} else if o.File != "" {
				b, err := ioutil.ReadFile(o.File)
				if err != nil {
					die(fmt.Sprintf("%s: %v", o.File, err))
				}
				query = string(b)
			} else {
				query = strings.Join(ra, " ")
			}
			s, err := sql.Parse(nil, query)
			if err != nil {
				die(fmt.Sprintf("parse: %v", err))
			}
			fmt.Printf("%+v\n", s)
			return 0
		},
	}
}
