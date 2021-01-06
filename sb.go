package main

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"time"

	"github.com/jessevdk/go-flags"
)

func main() {
	if len(os.Args) == 1 {
		die("specify subcommand or -h")
	}
	if os.Args[1] == "-h" {
		usages()
	}
	if f, ok := funcs[os.Args[1]]; ok {
		os.Exit(f.f(os.Args[2:]))
	}
	die("unknown subcommand")
}

func die(m string) {
	fmt.Fprintln(os.Stderr, m)
	os.Exit(1)
}

type subcommand struct {
	usage   string
	summary string
	f       func([]string) int
}

func usages() {
	fmt.Println(`sb (sandbox) commands:`)
	keys := make([]string, len(funcs))
	i := 0
	for n := range funcs {
		keys[i] = n
		i++
	}
	sort.Strings(keys)
	for _, n := range keys {
		c := funcs[n]
		fmt.Printf("%s %s\n  %s\n", n, c.usage, c.summary)
	}
	os.Exit(0)
}

var funcs = map[string]subcommand{
	"epoch": {
		"[--from-base=10] [--from=<time>] [--base=10]",
		"Prints seconds since UNIX Epoch of current or given time, converting base",
		func(a []string) int {
			o := struct {
				Base     int `long:"base"`
				FromTime FromTime
			}{
				Base:     10,
				FromTime: FromTime{FromBase: 10},
			}
			p := flags.NewParser(&o, 0)
			_, err := p.ParseArgs(a)
			if err != nil {
				die(fmt.Sprintf("parse: %v", err))
			}
			t := o.FromTime.OrNow()
			fmt.Println(big.NewInt(t.Unix()).Text(o.Base))
			return 0
		}},
}

type FromTime struct {
	FromBase int    `long:"from-base"`
	From     string `long:"from"`
}

func (f FromTime) OrNow() time.Time {
	if f.From == "" {
		return time.Now()
	}
	i := &big.Int{}
	i, err := i.SetString(f.From, f.FromBase)
	if i == nil {
		die(fmt.Sprintf("parse: %v", err))
	}
	return time.Unix(i.Int64(), 0)
}
