package parse_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type Input string

func (i *Input) expectHWS() bool {
	*i = Input(strings.TrimLeft(string(*i), " \t"))
	return true
}

func (i *Input) copy() *Input {
	ni := *i
	return &ni
}

func (i *Input) expect(prefix string) bool {
	if strings.HasPrefix(string(*i), prefix) {
		*i = Input(string(*i)[len(prefix):])
		return true
	}
	return false
}

func (i *Input) expectCI(prefix string) bool {
	if strings.HasPrefix(strings.ToLower(string(*i)),
		strings.ToLower(prefix)) {
		*i = Input(string(*i)[len(prefix):])
		return true
	}
	return false
}
func (i *Input) ended() bool {
	return len(string(*i)) == 0
}

func (i *Input) match(re string, res *[]string) bool {
	//fmt.Printf("checking %s\n", string(*i))
	//fmt.Printf("against %s\n", re)
	sm := regexp.MustCompile("^" + re).FindStringSubmatch(string(*i))
	//fmt.Printf("FindStringSubmatch returned %+v\n", sm)
	if len(sm) == 0 {
		return false
	}
	*i = Input(string(*i)[len(sm[0]):])
	*res = sm
	return true
}

type Link struct {
	Text []Inline
	URL  string
}

type Code string
type Text string

func (i *Input) ParseLink(l *Link) bool {
	b := i.copy()
	if !b.expect("[") {
		return false
	}
	*l = Link{}
	if !b.parseInlines(']', &l.Text) {
		return false
	}
	if !b.expect("](") {
		return false
	}
	var sub []string
	if !b.match(`([^)]+)`, &sub) {
		return false
	}
	if !b.expect(")") {
		return false
	}
	l.URL = sub[1]
	*i = *b
	return true
}

func TestLinkWithCode(t *testing.T) {
	i := Input("[here is a link with `code` in the middle](https://github.com)\n")
	var l Link
	require.True(t, i.ParseLink(&l))
	require.Equal(t, Link{
		Text: []Inline{
			Text("here is a link with "),
			Code("code"),
			Text(" in the middle"),
		},
		URL: "https://github.com",
	}, l)

}

type Table struct {
	Headers TableRow
	Rows    []TableRow
}
type TableRow []TableCell
type TableCell []Inline

func (i *Input) ParseTable() *Table {
	t := Table{}
	var r TableRow
	if !i.parseRow(&r) {
		return nil
	}
	t.Headers = r
	if !i.parseRow(&r) {
		return &t
	}
	if !isDelimiterRow(r) {
		t.Rows = append(t.Rows, r)
	}
	for i.parseRow(&r) {
		t.Rows = append(t.Rows, r)
	}

	return &t
}

func (i *Input) parseRow(r *TableRow) bool {
	var cells []TableCell
	b := i.copy()
	if !b.expect("|") {
		return false
	}
	for {
		var inlines []Inline
		b.parseInlines('|', &inlines)
		if !b.expect("|") {
			return false
		}
		cells = append(cells, TableCell(inlines))
		b.expectHWS()
		if b.expect("\n") || b.ended() {
			break
		}
	}
	*r = TableRow(cells)
	*i = *b
	return true
}

type Inline interface{}

func (i *Input) parseInlines(delim byte, inlines *[]Inline) bool {
	first := true
	for {
		var inline Inline
		if !i.parseInline(delim, &inline) {
			break
		}
		if first {
			t, ok := inline.(Text)
			if ok {
				trimmed := strings.TrimLeft(string(t), " ")
				if len(trimmed) == 0 {
					continue
				}
				inline = Text(trimmed)
			}
			first = false
		}
		*inlines = append(*inlines, inline)
	}

	if len(*inlines) != 0 {
		last := &(*inlines)[len(*inlines)-1]
		if t, ok := (*last).(Text); ok {
			trimmed := strings.TrimRight(string(t), " ")
			if len(trimmed) == 0 {
				*inlines = (*inlines)[:len(*inlines)-1]
				return true
			}
			*last = Text(trimmed)
		}
	}
	return true
}

func (i *Input) parseInline(delim byte, inline *Inline) bool {
	if i.parseCode(inline) {
		return true
	}
	var sub []string
	if i.match("[^`\\n\\"+string(delim)+"]+", &sub) {
		*inline = Text(sub[0])
		return true
	}
	return false
}

func (i *Input) parseCode(res *Inline) bool {
	var sub []string
	b := i.copy()
	if !b.match("`+|~+", &sub) {
		return false
	}
	backtickString := sub[0]
	end := 0
	s := string(*b)
	for {
		bo := strings.Index(s[end:], backtickString)
		if bo < 0 {
			return false
		}
		end += bo + len(backtickString)
		if len(s) == end || s[end] != backtickString[0] {
			break
		}
		for len(s) > end && s[end] == backtickString[0] {
			end++
		}
	}
	trim := s[:end-len(backtickString)]
	if strings.TrimSpace(trim) != "" &&
		trim[0] == ' ' &&
		trim[len(trim)-1] == ' ' {
		trim = trim[1 : len(trim)-1]
	}

	*res = Code(trim)
	*i = Input(s[end:])
	return true
}

func TestCode(t *testing.T) {
	var inline Inline
	i := Input("`hello`")
	require.True(t, i.parseCode(&inline))
	require.Equal(t, "hello", string(inline.(Code)))
	i = Input("`  ``  `")
	require.True(t, i.parseCode(&inline))
	require.Equal(t, " `` ", string(inline.(Code)))
	i = Input("`    `")
	require.True(t, i.parseCode(&inline))
	require.Equal(t, "    ", string(inline.(Code)))
	i = Input("` foo `` bar ` goo`")
	require.True(t, i.parseCode(&inline))
	require.Equal(t, "foo `` bar", string(inline.(Code)))
	require.Equal(t, " goo`", string(i))
}

func isDelimiterRow(r TableRow) bool {
	for _, c := range r {
		if len(c) > 1 {
			return false
		}
		if len(c) == 0 {
			continue
		}
		x, ok := c[0].(Text)
		if !ok || strings.TrimLeft(string(x), "-") != "" {
			return false
		}
	}
	return true
}

func TestTable(t *testing.T) {
	i := Input("| Key type | Bits in `ssh` key |  \n" +
		"| -------- | ---------------- |            \n" +
		//TODO"| :-------- | ----------------: |            \n" +
		"| RSA | ∞ |\n" +
		"| `ed25519` | 256 |")
	table := i.ParseTable()
	require.Equal(t, &Table{
		Headers: []TableCell{
			{Text("Key type")},
			{Text("Bits in "), Code("ssh"), Text(" key")},
		},
		Rows: []TableRow{
			{{Text("RSA")}, {Text("∞")}},
			{{Code("ed25519")}, {Text("256")}},
		},
	}, table)

}
