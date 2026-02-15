//go:build libmagic

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
	"github.com/wenerme/tools/pkg/libmagic"
)

func init() {
	funcs["magic"] = subcommand{
		`[files...]`,
		"analyzes files (or stdin) with system libmagic and outputs MIME type",
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
			if err := Magic(files); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func Magic(files []string) error {
	mgc := libmagic.Open(libmagic.MAGIC_NONE)
	defer mgc.Close()
	err := mgc.Load("")
	if err != nil {
		return fmt.Errorf("magic: %w", err)
	}
	//fmt.Printf("file: %s", mgc.File(os.Args[0]))
	mgc.SetFlags(libmagic.MAGIC_MIME | libmagic.MAGIC_MIME_ENCODING)

	if len(files) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		kind := mgc.Buffer(b)
		fmt.Println(kind)
	}

	for i := range files {
		fmt.Printf("%s: %s", files[i], mgc.File(files[i]))
	}

	return nil
}
