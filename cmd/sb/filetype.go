package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/h2non/filetype"
	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["filetype"] = subcommand{
		`[files...]`,
		"analyzes files (or stdin) and outputs MIME type",
		func(a []string) int {
			o := struct{}{}
			p := flags.NewParser(&o, 0)
			files, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if err := FileType(files); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func FileType(files []string) error {

	if len(files) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		kind, err := filetype.Match(b)
		if err != nil {
			return fmt.Errorf("filetype: %w", err)
		}
		fmt.Println(kind.MIME.Value)
	}

	for i := range files {
		b, err := os.ReadFile(files[i])
		if err != nil {
			return fmt.Errorf("read: %s: %w", files[i], err)
		}
		kind, err := filetype.Match(b)
		if err != nil {
			return fmt.Errorf("filetype: %s: %w", files[i], err)
		}
		if len(files) > 1 {
			fmt.Printf("%s: %+v\n", files[i], kind.MIME.Value)
		} else {
			fmt.Println(kind.MIME.Value)
		}
	}

	return nil
}
