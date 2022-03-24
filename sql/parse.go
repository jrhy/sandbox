package sql

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jrhy/sandbox/sql/colval"
)

type Expression struct {
	Input      string
	LastReject string
	Select     *Select `json:",omitempty"`
	Values     *Values `json:",omitempty"`
}

type Select struct {
	With        []With             `json:",omitempty"` // TODO generalize With,Values,Tables to FromItems
	Expressions []OutputExpression `json:",omitempty"`
	Tables      []Table            `json:",omitempty"`
	Values      []Values           `json:",omitempty"`
	Where       []Condition        `json:",omitempty"`
	Join        *Join              `json:",omitempty"`
	SetOp       *SetOp             `json:",omitempty"`
	Schema      Schema             `json:",omitempty"`
}

type JoinType int

const (
	InnerJoin = JoinType(iota)
	LeftJoin
	OuterJoin
)

type Join struct {
	JoinType JoinType `json:",omitempty"`
	Right    string   `json:",omitempty"`
	Using    string   `json:",omitempty"`
	//On Condition
}

type Values struct {
	Rows   []Row   `json:",omitempty"`
	Schema *Schema `json:",omitempty"`
}

type Row []ColumnValue

type ColumnValue interface {
	String() string
}

type OutputExpression struct {
	Expression SelectExpression `json:",omitempty"`
	Alias      string           `json:",omitempty"`
}

type SelectExpression struct {
	Column *Column `json:",omitempty"`
	Func   *Func   `json:",omitempty"`
}

type Column struct {
	//Family Family
	Term string
	All  bool
}

type Func struct {
	Aggregate bool
	Name      string
	RowFunc   func(*Row) ColumnValue
	//Expression OutputExpression //TODO: should maybe SelectExpression+*
}

type Table struct {
	Schema string `json:",omitempty"`
	Table  string `json:",omitempty"`
	Alias  string `json:",omitempty"`
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
	if !strings.HasPrefix(re.String(), "^") {
		panic("regexp missing ^ restriction: " + re.String())
	}
	return func(b *Expression) bool {
		s := re.FindStringSubmatch(b.Input)
		if s != nil && submatchcb != nil && submatchcb(s) {
			b.Input = b.Input[len(s[0]):]
			return true
		}
		return false
	}
}

var stringValueRE = regexp.MustCompile(`^'([^']*)'`)
var intValueRE = regexp.MustCompile(`^\d+`)
var realValueRE = regexp.MustCompile(`^(((\+|-)?([0-9]+)(\.[0-9]+)?)|((\+|-)?\.?[0-9]+))`)

func values(values **Values) parseFunc {
	var v *Values
	var schema Schema
	addCol := func(cv ColumnValue) {
		n := len(v.Rows) - 1
		v.Rows[n] = append(v.Rows[n], cv)
		if v.Schema != nil {
			return
		}
		schema.Columns = append(schema.Columns,
			SchemaColumn{Name: fmt.Sprintf("column%d", len(schema.Columns)+1)})
	}
	return SeqWS(
		CI("values").Action(func() { v = &Values{} }),
		Delimited(SeqWS(
			Exact("(").Action(func() {
				v.Rows = append(v.Rows, nil)
			}),
			Delimited(
				SeqWS(OneOf(
					RE(stringValueRE, func(s []string) bool {
						addCol(colval.Text(s[1]))
						return true
					}),
					RE(intValueRE, func(s []string) bool {
						i, err := strconv.ParseInt(s[0], 0, 64)
						if err != nil {
							return false
						}
						addCol(colval.Int(i))
						return true
					}),
					RE(realValueRE, func(s []string) bool {
						f, err := strconv.ParseFloat(s[0], 64)
						if err != nil {
							return false
						}
						addCol(colval.Real(f))
						return true
					}),
					CI("null").Action(func() {
						addCol(colval.Null{})
					}),
				)),
				Exact(","),
			),
			Exact(")").Action(func() {
				if v.Schema == nil {
					v.Schema = &schema
				}
			}),
		),
			Exact(",")),
	).Action(func() { *values = v })
}

func (b *Expression) Parse() bool {
	var s Select
	var v *Values
	return b.match(SeqWS(
		OneOf(
			SeqWS(Optional(SeqWS(CI("with"), with(&s.With))),
				ParseSelect(&s)),
			values(&v),
		),
		Optional(Exact(";")),
	).Action(func() {
		b.Select = &s
		b.Values = v
	}))
}

func ParseSelect(s *Select) parseFunc {
	return SeqWS(
		CI("select"),
		outputExpressions(&s.Expressions),
		Optional(SeqWS(
			CI("from"),
			fromItems(s),
		)),
		Optional(where(&s.Where)),
		Optional(setOp(&s.SetOp)).Action(func() {
			var schema int
			uniqName := func() string {
				for {
					name := fmt.Sprintf("schema%d", schema)
					if _, ok := s.Schema.Children[name]; !ok {
						return name
					}
					schema++
				}
			}
			if s.Schema.Children == nil {
				s.Schema.Children = make(map[string]*Schema)
			}
			for _, w := range s.With {
				name := w.Name
				if name == "" {
					name = uniqName()
				}
				if w.Values != nil {
					s.Schema.Children[name] = w.Values.Schema
				} else {
					s.Schema.Children[name] = &w.Select.Schema
				}
			}
			for _, v := range s.Values {
				s.Schema.Children[uniqName()] = v.Schema
			}
			if s.Join != nil {
				panic("child schema resolution unimpl for join")
			}
			for _, t := range s.Tables {
				name := t.Alias
				if name == "" {
					name = t.Table
				}
				_, found := s.Schema.Children[name]
				if !found {
					panic("how to handle fromItem resolution failure")
				}
			}
			for _, o := range s.Expressions {
				name := o.Alias
				if o.Expression.Func == nil && o.Expression.Column == nil {
					panic("non-Column unimpl")
				}
				if o.Expression.Func != nil {
					if o.Alias != "" {
						s.Schema.Columns = append(s.Schema.Columns, SchemaColumn{Name: o.Alias})
					} else {
						s.Schema.Columns = append(s.Schema.Columns, SchemaColumn{Name: o.Expression.Func.Name})
					}
				} else if o.Expression.Column.All {
					for _, t := range s.Tables {
						var schema *Schema
						var found bool
						{
							name := t.Alias
							if name == "" {
								name = t.Table
							}
							schema, found = s.Schema.Children[name]
							if !found {
								panic("how to handle fromItem resolution failure")
							}
						}
						for _, c := range schema.Columns {
							s.Schema.Columns = append(s.Schema.Columns, c)
						}
					}
				} else if name == "" {
					name = o.Expression.Column.Term
					panic("resolving specific column from schema unimpl")
				}
			}
		}),
	)
}

type Condition struct {
	Left  string `json:",omitempty"` // TODO not just columns but functions
	Right string `json:",omitempty"`
	Op    string `json:",omitempty"`
}

func where(conditions *[]Condition) parseFunc {
	return func(b *Expression) bool {
		var c *Condition
		return b.match(SeqWS(
			CI("where"),
			Delimited(SeqWS(
				condition(&c).Action(func() {
					*conditions = append(*conditions, *c)
				})),
				Exact("and")),
		))
	}
}
func condition(c **Condition) parseFunc {
	return func(b *Expression) bool {
		var left, right, op string
		return b.match(OneOf(
			// TODO: functions, fix sqlFunc, refactor w/SelectExpression
			SeqWS(name(&left), binaryOp(&op), name(&right)).Action(func() {
				*c = &Condition{Left: left, Right: right, Op: op}
			}),
		))
	}
}
func binaryOp(res *string) parseFunc {
	var Exact = func(s string, res *string) parseFunc {
		return Exact(s).Action(func() { *res = s })
	}
	return OneOf(
		Exact("==", res),
		Exact("is", res),
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

type SetOpType int

const (
	Union = SetOpType(iota)
	Intersect
	Except
)

type SetOp struct {
	Op    SetOpType `json:",omitempty"`
	All   bool      `json:",omitempty"`
	Right *Select   `json:",omitempty"`
}

func setOp(s **SetOp) parseFunc {
	return func(b *Expression) bool {
		var operation SetOpType
		var all bool
		var right Select
		return b.match(SeqWS(
			OneOf(
				CI("union").Action(func() { operation = Union }),
				CI("intersect").Action(func() { operation = Intersect }),
				CI("except").Action(func() { operation = Except })),
			Optional(CI("all").Action(func() { all = true })),
			ParseSelect(&right).Action(func() {
				*s = &SetOp{
					Op:    operation,
					All:   all,
					Right: &right,
				}
			})))
	}
}

type With struct {
	Name    string   `json:",omitempty"`
	Columns []string `json:",omitempty"`
	Values  *Values  `json:",omitempty"`
	Select  *Select  `json:",omitempty"`
}

func with(with *[]With) parseFunc {
	return func(b *Expression) bool {
		var w With

		return b.match(Delimited(SeqWS(
			RE(sqlNameRE, func(s []string) bool {
				w.Name = s[0]
				return true
			}),
			Optional(SeqWS(
				Exact("("),
				Delimited(
					SeqWS(RE(sqlNameRE, func(s []string) bool {
						w.Columns = append(w.Columns, s[0])
						return true
					})),
					Exact(","),
				),
				Exact(")"))),
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
			SeqWS(outputExpression(&e).Action(func() {
				*expressions = append(*expressions, e)
			})),
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
		//	var name string
		//	var e OutputExpression
		return b.match(OneOf(
			SeqWS(CI("current_timestamp")).Action(func() {
				*f = Func{Name: "current_timestamp",
					RowFunc: func(_ *Row) ColumnValue {
						return colval.Text(time.Now().UTC().Format("2006-01-02 15:04:05"))
					}}
			}),

			/*			SeqWS(
							sqlName(&name),
							Exact("("),
							outputExpression(&e),
							Exact(")"),
						).Action(func() {
							*f = Func{
								Name:       name,
								Expression: e,
							}
						}),*/
		),
		)

	}
}

func fromItems(s *Select) parseFunc {
	var v *Values
	return SeqWS(
		Delimited(OneOf(
			nameAs(func(name, as string) {
				s.Tables = append(s.Tables, Table{Table: name, Alias: as})
			}),
			SeqWS(
				Exact("("),
				values(&v),
				Exact(")").Action(func() {
					s.Values = append(s.Values, *v)
					s.Schema.Columns = append(s.Schema.Columns, v.Schema.Columns...)
				})),
		), Exact(",")),
		Optional(join(&s.Join)))
}

func name(res *string) parseFunc {
	return func(b *Expression) bool {
		var name string
		return b.match(SeqWS(
			sqlName(&name), // TODO: '.' should be broken out into separate schema
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

var sqlNameRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z_0-9-\.]*`)

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
	if e.Parse() == false {
		if e.Input != "" {
			return nil, errors.New("parse error at " + e.Input)
		}
		return nil, errors.New("parse error")
	}

	return e, nil
}
