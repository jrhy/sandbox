package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/jessevdk/go-flags"
)

type hgrepOptions struct {
	URL       string   `short:"u"`
	File      string   `short:"f"`
	Selectors []string `short:"o"`
	Invert    []string `short:"v"`
	Text      bool     `short:"t" long:"text"`
}

func init() {
	funcs["hgrep"] = subcommand{
		"[-t/--text] [-v <selector>]... [-o <selector>]...  [-u <url> | -f <path>|-]",
		"HTML grep applies jquery-style selector matching/filtering to the given document",
		func(a []string) int {
			o := hgrepOptions{}
			p := flags.NewParser(&o, 0)
			ra, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if o.File == "" && o.URL == "" || len(ra) > 0 {
				return exitSubcommandUsage
			}
			if err := scrape(o); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func scrape(o hgrepOptions) error {
	var err error
	var doc *goquery.Document
	if o.File == "-" || strings.HasPrefix(o.URL, "file:") {
		var r io.Reader
		if o.File == "-" || o.URL == "file:-" {
			r = os.Stdin
		} else {
			r, err = os.Open(strings.TrimPrefix(o.URL, "file:"))
			if err != nil {
				return err
			}
		}
		doc, err = goquery.NewDocumentFromReader(r)
		if err != nil {
			return err
		}
	} else {
		doc, err = goquery.NewDocument(o.URL)
		if err != nil {
			return err
		}
	}
	return scrapeDoc(o, doc)
}

func scrapeDoc(o hgrepOptions, doc *goquery.Document) (err error) {
	var matches *goquery.Selection
	if len(o.Selectors) != 0 {
		for _, s := range o.Selectors {
			if matches != nil {
				matches = matches.Union(doc.Find(s))
			} else {
				matches = doc.Find(s)
			}
		}
	} else {
		matches = doc.Selection
	}
	for _, s := range o.Invert {
		matches.Find(s).Remove()
	}
	if o.Text {
		os.Stdout.Write([]byte(matches.Text()))
		return nil
	}

	var res string
	matches.Each(func(_ int, s *goquery.Selection) {
		if err != nil {
			return
		}
		var html string
		html, err = goquery.OuterHtml(s)
		if err != nil {
			err = fmt.Errorf("html: %w", err)
			return
		}
		res += html
	})
	if err != nil {
		return err
	}
	os.Stdout.Write([]byte(res))
	return nil
}
