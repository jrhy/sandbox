package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["mimetype"] = subcommand{
		``,
		"analyzes stdin and outputs MIME type",
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
			if err := mimetype(); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func mimetype() error {
	b := make([]byte, 512)
	n, err := os.Stdin.Read(b)
	if n == 0 && err != nil {
		return err
	}
	fmt.Println(http.DetectContentType(b[:n]))
	return nil
}
