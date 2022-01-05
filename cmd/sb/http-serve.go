package main

import (
	"fmt"
	"net/http"
)

func httpServer(addr string) error {
	fs := http.FileServer(http.Dir("."))
	http.Handle("/", fs)
	fmt.Printf("listening on %s\n", addr)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		return fmt.Errorf("listenAndServe %s: %w", addr, err)
	}
	return nil
}

func init() {
	funcs["http-serve"] = subcommand{
		`TODO: -p <addr>`,
		"serves the current directory over HTTP on :34985",
		func(a []string) int {
			if err := httpServer(":34985"); err != nil {
				die(fmt.Sprintf("http-bench: %v", err))
			}
			return 0
		},
	}
}
