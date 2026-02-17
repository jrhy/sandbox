package main

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/mdp/qrterminal/v3"
)

const (
	exitError           = 1
	exitSubcommandUsage = 2
)

func main() {
	if len(os.Args) == 1 {
		die("specify subcommand or -h")
	}
	if os.Args[1] == "-h" {
		usages()
	}
	if f, ok := funcs[os.Args[1]]; ok {
		exit := f.f(os.Args[2:])
		if exit == exitSubcommandUsage {
			printSubcommandUsage(os.Args[1], f)
		}
		os.Exit(exit)
	}
	die("unknown subcommand")
}

func die(m string) {
	fmt.Fprintln(os.Stderr, m)
	os.Exit(1)
}

type subcommand struct {
	// usage is the command-specific CLI synopsis shown after "sb <name>".
	// It may be single-line or multiline. Keep additional lines indented for
	// readable "sb -h" output.
	usage string
	// summary is a short one-line description shown in "sb -h" listings.
	summary string
	f       func([]string) int
}

func printSubcommandUsage(name string, c subcommand) {
	fmt.Fprintf(os.Stderr, "usage: sb %s %s\n", name, c.usage)
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
	"qr": {
		"[-m/--message=<string>]",
		"Prints the QR code for the given message",
		func(a []string) int {
			o := struct {
				Message string `long:"message" short:"m"`
			}{}
			p := flags.NewParser(&o, 0)
			_, err := p.ParseArgs(a)
			if err != nil {
				die(fmt.Sprintf("parse: %v", err))
			}
			if o.Message == "" {
				return exitSubcommandUsage
			}
			qrterminal.Generate(o.Message, qrterminal.L, os.Stdout)
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
