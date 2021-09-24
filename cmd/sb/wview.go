package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/jessevdk/go-flags"
	"golang.org/x/net/html"

	"github.com/jrhy/sandbox/geminize"
)

func init() {
	funcs["wview"] = subcommand{
		``,
		"renders HTML input as a simplified text document",
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
			if err := doit(); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func doit() error {
	doc, err := goquery.NewDocumentFromReader(os.Stdin)
	if err != nil {
		return fmt.Errorf("goquery: %w", err)
	}
	u, err := url.Parse("file:-")
	if err != nil {
		return fmt.Errorf("internal url: %w", err)
	}
	geminize.Geminize(geminize.Options{}, doc, u)
	body := doc.Find("body")
	if body == nil {
		return nil
	}
	for _, n := range body.Nodes {
		printNodeText(n)
	}
	return nil
}

func printNodeText(n *html.Node) {
	var href string
	if n.Type == html.ElementNode {
		switch n.Data {
		case "body", "p", "b", "div":
		case "a":
			for i := range n.Attr {
				if n.Attr[i].Key == "href" {
					href = n.Attr[i].Val
					break
				}
			}
			if href != "" {
				fmt.Printf("\n\nLINK %s ", href)
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			fmt.Printf("\n\n= ")
		default:
			fmt.Printf("haha %s { \n", n.Data)
		}
	}
	if n.Type == html.TextNode {
		fmt.Printf("%s", n.Data)
	}
	if n.FirstChild != nil {
		printNodeText(n.FirstChild)
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "body", "b", "div":
		case "a":
			if href != "" {
				fmt.Printf("\n\n")
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			fmt.Printf(" =\n\n")
		case "p":
			fmt.Printf("\n\n")
		default:
			fmt.Printf("} // %s\n", n.Data)
		}
	}
	if false && n.Type == html.ElementNode {
		fmt.Printf("} // %s\n", n.Data)
	}
	if n.NextSibling != nil {
		printNodeText(n.NextSibling)
	}

}
