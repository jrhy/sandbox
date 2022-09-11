package main

import (
	"crypto/tls"
	"fmt"
	"strings"

	"golang.org/x/crypto/acme/autocert"

	"github.com/jessevdk/go-flags"
)

func init() {
	funcs["autocert"] = subcommand{
		`-l=example.com:443 --autocert-cache=<path> --autocert-name=example.com`,
		"one-shot HTTPS server that answers LetsEncrypt ALPN challenge and stores cert for given hostname",
		func(a []string) int {
			o := options{}
			p := flags.NewParser(&o, 0)
			_, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if o.Address == "" || o.AutocertCache == "" || o.AutocertName == "" {
				return exitSubcommandUsage
			}
			if err := runAutocert(o); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

type options struct {
	Address       string `short:"l"`
	AutocertName  string `long:"autocert-name"`
	AutocertCache string `long:"autocert-cache"`
}

func runAutocert(o options) error {
	m := &autocert.Manager{
		Cache:      autocert.DirCache(o.AutocertCache),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(o.AutocertName),
	}
	m.GetCertificate(&tls.ClientHelloInfo{
		ServerName: o.AutocertName,
	})
	return nil
}
