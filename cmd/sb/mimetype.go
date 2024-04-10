package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["mimetype"] = subcommand{
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
			mimetype.SetLimit(0)
			if err := MimeType(files); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func MimeType(files []string) error {

	if len(files) == 0 {
		mtype, err := mimetype.DetectReader(os.Stdin)
		if err != nil {
			return err
		}
		fmt.Println(mtype)
	}

	for i := range files {
		mtype, err := mimetype.DetectFile(files[i])
		if err != nil {
			return fmt.Errorf("%s: %w", files[i], err)
		}
		if len(files) > 1 {
			fmt.Printf("%s: %s\n", files[i], mtype)
		} else {
			fmt.Println(mtype)
		}
	}

	return nil
}
