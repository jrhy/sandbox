package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/jeremywohl/flatten"
	"github.com/jessevdk/go-flags"
	"golang.org/x/exp/maps"
)

func init() {
	funcs["flat"] = subcommand{
		``,
		"flattens (un-nests) JSON from input",
		func(a []string) int {
			o := struct{}{}
			p := flags.NewParser(&o, 0)
			args, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if len(args) > 0 {
				return exitSubcommandUsage
			}
			if err := FlattenJSON(); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func FlattenJSON() error {

	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	var nested map[string]interface{}
	err = json.Unmarshal(b, &nested)
	if err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	flat, err := flatten.Flatten(nested, "", flatten.DotStyle)
	if err != nil {
		return fmt.Errorf("flatten: %w", err)
	}
	keys := maps.Keys(flat)
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s: %v\n", k, flat[k])
	}

	return nil
}
