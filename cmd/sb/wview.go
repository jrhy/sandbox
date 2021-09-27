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

type state struct {
	Indent    string
	LineBreak bool
	ParaBreak bool
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
	s := state{}
	for _, n := range body.Nodes {
		s.printNodeText(n)
	}
	return nil
}

func (s *state) printNodeText(n *html.Node) {
	oldIndent := s.Indent
	var href string

	if n.Type == html.ElementNode {
		switch n.Data {
		case "body", "p", "b", "strong", "div", "span", "br":
		case "a":
			for i := range n.Attr {
				if n.Attr[i].Key == "href" {
					href = n.Attr[i].Val
					break
				}
			}
			if href != "" {
				s.lineBreak()
				s.emit(fmt.Sprintf("LINK %s\n", href))
				s.lineBreak()
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			s.paraBreak()
			s.emit("= ")
		case "table", "tbody", "tr", "td", "th":
		case "ul":
		case "li":
			s.lineBreak()
			s.emit(s.Indent)
		default:
			s.emit(fmt.Sprintf("haha %s { \n", n.Data))
		}
	}
	if n.Type == html.TextNode {
		s.emit(n.Data)
	}
	if n.FirstChild != nil {
		s.printNodeText(n.FirstChild)
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "body", "b", "strong", "div", "span", "li":
		case "a":
			if href != "" {
				s.lineBreak()
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			s.emit(" =")
			s.paraBreak()
		case "br":
			s.lineBreak()
		case "p":
			s.paraBreak()
		case "table":
			s.paraBreak()
		case "tr":
			s.lineBreak()
		case "tbody", "td", "th":
			//s.emit("  ")
		case "ul":
			s.lineBreak()
		default:
			s.emit(fmt.Sprintf("} // %s", n.Data))
			s.lineBreak()
		}
	}
	if n.NextSibling != nil {
		s.printNodeText(n.NextSibling)
	}
	s.Indent = oldIndent
}

func (s *state) lineBreak() {
	if s.LineBreak || s.ParaBreak {
		return
	}
	fmt.Printf("\n")
	s.LineBreak = true
}

func (s *state) paraBreak() {
	if s.ParaBreak {
		return
	}
	if s.LineBreak {
		fmt.Printf("\n")
	} else {
		fmt.Printf("\n\n")
	}
	s.ParaBreak = true
}

func (s *state) emit(st string) {
	noNL := strings.ReplaceAll(st, "\n", " ")
	noNL = strings.TrimSpace(noNL)
	if noNL == "" {
		return
	}
	fmt.Printf("%s ", noNL)
	s.LineBreak = false
	s.ParaBreak = false
}
