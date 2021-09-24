package geminize

import (
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/require"
)

func TestGeminize(t *testing.T) {
	t.Run("img_becomes_a", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(
			`<body><h1>hi</h1>
<img src='goo.gif' alt='haha'>
`))
		require.NoError(t, err)
		pageURL, err := url.Parse("http://example.com")
		require.NoError(t, err)
		Geminize(Options{Address: "localhost:9999", myScheme: "http"}, doc, pageURL)
		html, err := doc.Html()
		require.NoError(t, err)
		require.Equal(t,
			`<html><body><h1>hi</h1>
<a href="http://localhost:9999/example.com/goo.gif" rel="noopener noreferrer">Image &#34;haha&#34;</a>
</body></html>`,
			html)
	})

	t.Run("a_rel_replaced", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(
			`<body><h1>hi</h1>
<a href="newtab.html" rel="trackmeorsomething">linkylink</a>
`))
		require.NoError(t, err)
		pageURL, err := url.Parse("http://example.com")
		require.NoError(t, err)
		Geminize(Options{Address: "localhost:9999", myScheme: "http"}, doc, pageURL)
		html, err := doc.Html()
		require.NoError(t, err)
		require.Equal(t,
			`<html><body><h1>hi</h1>
<a href="http://localhost:9999/example.com/newtab.html" rel="noopener noreferrer">linkylink</a>
</body></html>`,
			html)
	})

	t.Run("a_targets_removed", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(
			`<body><h1>hi</h1>
<a href="newtab.html" target="_blank">linkylink</a>
`))
		require.NoError(t, err)
		pageURL, err := url.Parse("http://example.com")
		require.NoError(t, err)
		Geminize(Options{Address: "localhost:9999", myScheme: "http"}, doc, pageURL)
		html, err := doc.Html()
		require.NoError(t, err)
		require.Equal(t,
			`<html><body><h1>hi</h1>
<a href="http://localhost:9999/example.com/newtab.html" rel="noopener noreferrer">linkylink</a>
</body></html>`,
			html)
	})

}
