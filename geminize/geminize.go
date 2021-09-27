package geminize

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/net/html"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/crypto/acme/autocert"
)

type Options struct {
	Address       string `short:"l"`
	AutocertName  string `long:"autocert-name"`
	AutocertCache string `long:"autocert-cache"`
	TLSCertPath   string `long:"tls-cert"`
	TLSKeyPath    string `long:"tls-key"`
	File          string `short:"f"`
	myScheme      string
}

func Run(o Options) error {
	if o.File != "" {
		f, err := os.Open(o.File)
		if err != nil {
			return fmt.Errorf("open %s: %w", o.File, err)
		}
		doc, err := goquery.NewDocumentFromReader(f)
		if err != nil {
			return fmt.Errorf("goquery: %w", err)
		}
		u, err := url.Parse("file:" + o.File)
		if err != nil {
			return fmt.Errorf("internal url: %w", err)
		}
		Geminize(o, doc, u)
		html, err := doc.Find("body").Html()
		if err != nil {
			return fmt.Errorf("html: %w", err)
		}
		os.Stdout.Write([]byte(html))
		return nil
	}
	http.HandleFunc("/style.css", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("content-type", "text/css")
		io.WriteString(w, STYLE)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.EscapedPath(), "/"+o.Address) {
			w.WriteHeader(404)
			return
		}
		pageURL, err := url.Parse("https:/" + req.URL.EscapedPath())
		if err != nil {
			http.NotFound(w, req)
			return
		}
		oreq, err := http.NewRequest("GET", pageURL.String(), nil)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		oreq.Header.Del("user-agent")
		r, err := http.DefaultClient.Do(oreq)
		if err != nil {
			w.WriteHeader(r.StatusCode)
			return
		}
		contentType := strings.ToLower(r.Header.Get("content-type"))
		switch {
		case strings.HasPrefix(contentType, "image/"):
			w.Header().Set("content-type", contentType)
			w.WriteHeader(r.StatusCode)
			io.Copy(w, r.Body)
			return
		case strings.HasPrefix(contentType, "text/html"):
		case contentType == "":
		default:
			w.Header().Set("content-type", "text/plain")
			w.WriteHeader(400)
			w.Write([]byte(fmt.Sprintf("not relaying content of type '%s'", contentType)))
			return
		}

		doc, err := goquery.NewDocumentFromResponse(r)
		if err != nil {
			w.Header().Set("content-type", "text/plain")
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}

		Geminize(o, doc, pageURL)
		html, err := doc.Find("body").Html()
		if err != nil {
			w.Header().Set("content-type", "text/plain")
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
		w.Header().Set("content-type", "text/html")
		io.WriteString(w, `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta name="viewport" content="width=device-width, initial-scale=1"/>
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
    <meta name="referrer" content="no-referrer" />
    <link href="/style.css" rel="stylesheet">`)
		io.WriteString(w, "</head><body>")
		io.WriteString(w, html)
	})

	if o.AutocertName == "" && o.TLSCertPath == "" {
		o.myScheme = "http"
		return http.ListenAndServe(o.Address, nil)
	}
	if o.TLSCertPath != "" {
		if o.TLSKeyPath == "" {
			return errors.New("need to set --tls-key-path to use TLS")
		}
		o.myScheme = "https"
		return http.ListenAndServeTLS(o.Address, o.TLSCertPath, o.TLSKeyPath, nil)
	}
	if o.TLSKeyPath != "" {
		return errors.New("need to set --tls-cert-path to use TLS")
	}

	if o.AutocertCache == "" {
		return errors.New("need to set --autocert-cache to use autocert")
	}
	if o.AutocertName == "" {
		return errors.New("need to set --autocert-name to use autocert")
	}
	m := &autocert.Manager{
		Cache:      autocert.DirCache(o.AutocertCache),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(o.AutocertName),
	}
	tlsConfig := m.TLSConfig()
	s := &http.Server{
		Addr:      o.Address,
		TLSConfig: tlsConfig,
	}
	l, err := net.Listen("tcp", o.Address)
	if err != nil {
		return fmt.Errorf("listen %s: %w", o.Address, err)
	}
	l = tls.NewListener(l, tlsConfig)
	o.myScheme = "https"
	return s.Serve(l)
}

const STYLE = `@media (prefers-color-scheme: dark) {
  body {
    background-color: black;
    color: #00ff00;
  }
  a:visited { color: gray; }
  a:link { color: white; }
  a:hover { color: hotpink; }
  a:active { color: hotpink; }
}
@media (prefers-color-scheme: light) {
  body {
    background-color: #FFFFFF;
    color: #000000;
  }
}
html {
  font-family: sans-serif;
}
p.c1 {
  font-size: 200%;
  text-align: center
}
.hangingindent {
  padding-left: 22px;
  text-indent: -22px;
}
.indent {
  text-indent: 22px;
}
.nomargin p {
  margin: 0;
}`

func Geminize(o Options, doc *goquery.Document, pageURL *url.URL) {
	doc.Find("script").Remove()
	doc.Find("style").Remove()
	doc.Find("noscript").Remove()
	doc.Find("link").Remove()
	doc.Find("picture").Remove()
	doc.Find("svg").Remove()
	doc.Find("polygon").Remove()
	doc.Find("nav").Remove()

	doc.Find("header").Remove()
	doc.Find(".mediaEmbed").Remove()
	doc.Find("div[latest_bottom_wrap]").Remove()
	doc.Find(".share").Remove()
	doc.Find(".similarLinks").Remove()
	doc.Find(".connect").Remove()
	doc.Find(".account").Remove()
	doc.Find(".contentFeedback").Remove()
	doc.Find(".comments").Remove()
	doc.Find(".newsletterWidget").Remove()
	doc.Find(".messaging-block").Remove()
	doc.Find(".comments-section").Remove()
	doc.Find(".presents-box").Remove()
	doc.Find(".popend").Remove()
	doc.Find("footer").Remove()
	doc.Find("figure").Each(func(i int, s *goquery.Selection) {
		img := s.Find("img")
		if img == nil {
			return
		}
		alt, _ := img.Attr("alt")
		if alt != "" {
			return
		}
		fc := s.Find("figcaption")
		if fc == nil {
			return
		}
		img.SetAttr("alt", fc.Text())
	})
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		alt, _ := s.Attr("alt")
		//s.RemoveAttr("src")
		//s.RemoveAttr("srcset")
		s.SetAttr("referrerpolicy", "no-referrer")

		node := &html.Node{
			Type: html.ElementNode,
			Data: "a",
			Attr: []html.Attribute{{
				Key: "href",
				Val: src,
			}},
		}
		imgComment := strings.TrimSpace(alt)
		if imgComment != "" {
			imgComment = `Image "` + imgComment + `"`
		} else {
			imgComment = "Image"
		}
		node.AppendChild(&html.Node{
			Type: html.TextNode,
			Data: imgComment,
		})
		/*
			var f func(*html.Node)
			indent := ""
			f = func(n *html.Node) {
				oldIndent := indent
				indent += "  "
				fmt.Printf("%stype:%v, data=%v\n", indent, n.Type, n.Data)
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					f(c)
				}
				indent = oldIndent
			}
			f(node)*/

		s.ReplaceWithNodes(node)
	})
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		dest, ok := s.Attr("href")
		if !ok {
			s.SetAttr("href", "")
			return
		}
		destURL, err := url.Parse(dest)
		if err != nil {
			s.SetAttr("href", "")
			return
		}
		destURL = pageURL.ResolveReference(destURL)
		s.SetAttr("href", o.myScheme+"://"+o.Address+"/"+destURL.Host+destURL.Path)
		s.SetAttr("rel", "noopener noreferrer")
	})
	doc.Find("a[target]").Each(func(i int, s *goquery.Selection) {
		s.RemoveAttr("target")
	})
	toKeep := doc.Find("body,div,p,table,tr,td,ul,li,a,h1,h2,h3,h4,h5,h6,i,b,u,strong,em,img,br")
	toKeep = toKeep.Union(toKeep.Parents())
	doc.Find("*").NotSelection(toKeep).Remove()
}
