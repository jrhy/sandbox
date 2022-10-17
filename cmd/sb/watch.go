package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jrhy/sandbox/command"
)

func init() {
	funcs["watch"] = subcommand{
		`[args]`,
		"reruns the given command every 2 seconds",
		func(a []string) int {
			if len(os.Args) < 2 {
				return exitSubcommandUsage
			}
			if err := watch(); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func watch() error {
	for {
		//fmt.Printf("[2J")
		cmd := command.New(os.Args[2], os.Args[3:]...)
		_, err := cmd.RelayOutput().Run()
		if err != nil {
			fmt.Printf("%v\n", err)
		}
		time.Sleep(2 * time.Second)
	}
}
