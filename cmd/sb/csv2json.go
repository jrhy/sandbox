package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["csv2json"] = subcommand{
		``,
		"converts CSV input to JSON",
		func(a []string) int {
			o := struct {
			}{}
			p := flags.NewParser(&o, 0)
			_, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if err := csv2json(); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func csv2json() error {
	csv := csv.NewReader(os.Stdin)
	header, err := csv.Read()
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	for {
		r, err := csv.Read()
		if err != nil && err != io.EOF {
			return fmt.Errorf("read: %w", err)
		}
		if r == nil {
			return nil
		}
		m := make(map[string]string, len(header))
		for i := range r {
			if i >= len(header) {
				break
			}
			m[header[i]] = r[i]
		}
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		fmt.Println(string(b))
	}
}
