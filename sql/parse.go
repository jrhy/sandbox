package sql

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Expression struct {
	Input      string
	LastReject string
	Select     *Select
	Values     *Values
}

type Select struct {
	With        []With
	Expressions []OutputExpression
	Tables      []Table
	Values      *Values
	Join        *Join
}

type JoinType int

const (
	InnerJoin = JoinType(iota)
	LeftJoin
	OuterJoin
)

type Join struct {
	JoinType JoinType
	Right    string
	Using    string
	//On Condition
}

type Values struct {
	Rows [][]ColumnValue
}

type OutputExpression struct {
	Expression SelectExpression
	Alias      string
}

type SelectExpression struct {
	Column *Column
	Func   *Func
}

type Column struct {
	//Family Family
	Term string
	All  bool
}

type Func struct {
	Aggregate  bool
	Name       string
	Expression OutputExpression //TODO: should maybe SelectExpression+*
}

type Table struct {
	Schema string
	Table  string
	Alias  string
}

func (b *Expression) copy() *Expression {
	ne := *b
	return &ne
}

func (b *Expression) ExactWS() bool {
	b.Input = strings.TrimLeft(b.Input, " \t\r\n")
	return true
}

func (b *Expression) Exact(prefix string) bool {
	if strings.HasPrefix(b.Input, prefix) {
		b.Input = b.Input[len(prefix):]
		return true
	}
	return false
}

func (b *Expression) CI(prefix string) bool {
	if strings.HasPrefix(strings.ToLower(b.Input),
		strings.ToLower(prefix)) {
		b.Input = b.Input[len(prefix):]
		return true
	}
	return false
}

func (b *Expression) String() string {
	bs, err := json.MarshalIndent(*b, "", " ")
	if err != nil {
		panic(err)
	}
	return string(bs)
}

type parseFunc func(*Expression) bool

func SeqWS(fns ...func(*Expression) bool) parseFunc {
	return func(b *Expression) bool {
		e := b.copy()
		for _, f := range fns {
			e.ExactWS()
			if !f(e) {
				if len(e.Input) < len(b.LastReject) {
					b.LastReject = e.Input
				}
				return false
			}
			e.ExactWS()
		}
		*b = *e
		return true
	}
}

func CI(s string) parseFunc {
	return func(b *Expression) bool {
		return b.CI(s)
	}
}

func Exact(s string) parseFunc {
	return func(b *Expression) bool {
		return b.Exact(s)
	}
}

func (b *Expression) match(f func(*Expression) bool) bool {
	return f(b)
}

func (m parseFunc) Action(then func()) parseFunc {
	return func(b *Expression) bool {
		if m(b) {
			then()
			return true
		}
		return false
	}
}

func (m parseFunc) Or(other parseFunc) parseFunc {
	return func(b *Expression) bool {
		if m(b) {
			return true
		}
		return other(b)
	}
}

func OneOf(fns ...func(*Expression) bool) parseFunc {
	return func(b *Expression) bool {
		for _, f := range fns {
			if f(b) {
				return true
			}
		}
		return false
	}
}

func Optional(f parseFunc) parseFunc {
	return func(b *Expression) bool {
		e := b.copy()
		if e.match(f) {
			*b = *e
		}
		return true
	}
}

func AtLeastOne(f parseFunc) parseFunc {
	return func(b *Expression) bool {
		e := b.copy()
		if !e.match(f) {
			return false
		}
		for e.match(f) {
		}
		*b = *e
		return true
	}
}

func RE(re *regexp.Regexp, submatchcb func([]string) bool) parseFunc {
	return func(b *Expression) bool {
		fmt.Printf("checking RE %v against '%v'\n", re, b.Input)
		s := re.FindStringSubmatch(b.Input)
		if s != nil && submatchcb != nil && submatchcb(s) {
			b.Input = b.Input[len(s[0]):]
			return true
		}
		return false
	}
}

type ColumnValue struct {
	Text    string
	Integer int64
	Real    float64
	Null    bool
}

var stringValueRE = regexp.MustCompile(`^'([^']*)'`)
var intValueRE = regexp.MustCompile(`^\d+`)
var realValueRE = regexp.MustCompile(`^(((\+|-)?([0-9]+)(\.[0-9]+)?)|((\+|-)?\.?[0-9]+))`)

// TODO: break out values, rows, columnValues
func columnValues(b *Expression) parseFunc {
	return values(&b.Values)
}

func values(values **Values) parseFunc {
	var v Values
	addCol := func(cv ColumnValue) {
		n := len(v.Rows) - 1
		v.Rows[n] = append(v.Rows[n], cv)
	}
	return SeqWS(
		CI("values"),
		Delimited(SeqWS(
			Exact("(").Action(func() {
				v.Rows = append(v.Rows, nil)
			}),
			Delimited(
				SeqWS(OneOf(
					RE(stringValueRE, func(s []string) bool {
						addCol(ColumnValue{Text: s[1]})
						return true
					}),
					RE(intValueRE, func(s []string) bool {
						i, err := strconv.ParseInt(s[0], 0, 64)
						if err != nil {
							return false
						}
						addCol(ColumnValue{Integer: i})
						return true
					}),
					RE(realValueRE, func(s []string) bool {
						f, err := strconv.ParseFloat(s[0], 64)
						if err != nil {
							return false
						}
						addCol(ColumnValue{Real: f})
						return true
					}),
					CI("null").Action(func() {
						addCol(ColumnValue{Null: true})
					}),
				)),
				Exact(","),
			),
			Exact(")"),
		),
			Exact(",")),
	).Action(func() { *values = &v })
}

func (b *Expression) Parse() bool {
	var s Select
	return b.match(SeqWS(
		OneOf(
			SeqWS(Optional(SeqWS(CI("with"), with(&s.With))),
				ParseSelect(&s)),
			columnValues(b),
		),
		Optional(Exact(";")),
	).Action(func() { b.Select = &s }))
}

func ParseSelect(s *Select) parseFunc {
	return SeqWS(
		CI("select"),
		outputExpressions(&s.Expressions),
		Optional(SeqWS(CI("from"), OneOf(
			tables(&s.Tables),
			SeqWS(Exact("("), values(&s.Values), Exact(")")),
		))),
		Optional(join(&s.Join)),
	)
}

func join(j **Join) parseFunc {
	return func(b *Expression) bool {
		var joinType JoinType = LeftJoin
		var using, tableName string
		return b.match(SeqWS(
			Optional(OneOf(
				CI("outer").Action(func() { joinType = OuterJoin }),
				CI("inner").Action(func() { joinType = InnerJoin }),
				CI("left").Action(func() { joinType = LeftJoin }))),
			CI("join"),
			name(&tableName),
			OneOf(
				SeqWS(CI("using"), Exact("("), name(&using), Exact(")")),
				//SeqWS(CI("on"), condition),
			)).Action(func() {
			*j = &Join{
				JoinType: joinType,
				Right:    tableName,
				Using:    using,
			}
		}))
	}
}

type With struct {
	Name    string
	Columns []string
	Values  *Values
	Select  *Select
}

func with(with *[]With) parseFunc {
	return func(b *Expression) bool {
		var w With

		return b.match(Delimited(SeqWS(
			RE(sqlNameRE, func(s []string) bool {
				w.Name = s[0]
				return true
			}),
			Exact("("),
			Exact("").Action(func() { fmt.Printf("hey yo bracket\n") }),
			Delimited(SeqWS(RE(sqlNameRE, func(s []string) bool {
				w.Columns = append(w.Columns, s[0])
				return true
			})), Exact(",")),
			Exact(")"),
			CI("as"),
			//ParseSelect(&w.Select),
			Exact("("),
			values(&w.Values),
			Exact(")"),
			Exact("").Action(func() {
				*with = append(*with, w)
			}),
		),
			Exact(",")))
	}
}

func outputExpressions(expressions *[]OutputExpression) parseFunc {
	return func(b *Expression) bool {
		var e OutputExpression
		return b.match(Delimited(
			outputExpression(&e).Action(func() {
				*expressions = append(*expressions, e)
			}),
			Exact(",")))
	}
}
func outputExpression(e *OutputExpression) parseFunc {
	return func(b *Expression) bool {
		var as string
		var f Func
		return b.match(OneOf(
			SeqWS(sqlFunc(&f), Optional(As(&as))).Action(func() {
				*e = OutputExpression{Expression: SelectExpression{Func: &f}, Alias: as}
			}),
			nameAs(func(name, as string) {
				*e = OutputExpression{Expression: SelectExpression{Column: &Column{Term: name}}, Alias: as}
			}),
			Exact("*").Action(func() {
				*e = OutputExpression{Expression: SelectExpression{Column: &Column{All: true}}}
			}),
		))
	}
}

func sqlFunc(f *Func) parseFunc {
	return func(b *Expression) bool {
		var name string
		var e OutputExpression
		return b.match(SeqWS(
			sqlName(&name),
			Exact("("),
			outputExpression(&e),
			Exact(")"),
		).Action(func() {
			*f = Func{
				Name:       name,
				Expression: e,
			}
		}))

	}
}

func tables(tables *[]Table) parseFunc {
	return func(b *Expression) bool {
		return b.match(Delimited(nameAs(func(name, as string) {
			*tables = append(*tables, Table{Table: name, Alias: as})
		}), Exact(",")))
	}
}

func name(res *string) parseFunc {
	return func(b *Expression) bool {
		var name string
		return b.match(SeqWS(
			sqlName(&name),
		).Action(func() { *res = name }))
	}
}

func nameAs(cb func(name, as string)) parseFunc {
	return func(b *Expression) bool {
		var name, as string
		return b.match(SeqWS(
			sqlName(&name),
			Optional(As(&as)),
		).Action(func() { cb(name, as) }))
	}
}

var sqlNameRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z_0-9-]*`)

func sqlName(res *string) parseFunc {
	return RE(sqlNameRE, func(s []string) bool {
		*res = s[0]
		return true
	})
}

func As(as *string) parseFunc {
	return SeqWS(
		CI("as"),
		RE(sqlNameRE, func(s []string) bool {
			*as = s[0]
			return true
		}))
}

func Delimited(
	term parseFunc, delimiter parseFunc) parseFunc {
	return func(e *Expression) bool {
		terms := 0
		for {
			if !term(e) {
				break
			}
			terms++
			if !delimiter(e) {
				break
			}
		}
		return terms > 0
	}
}

func Parse(s string) (*Expression, error) {
	e := &Expression{Input: s, LastReject: s}
	if e.Parse() {
		return e, nil
	}
	if e.Input != "" {
		return nil, errors.New("unparsed input starting at " + e.LastReject)
	}
	return nil, errors.New("not recognized")
}
