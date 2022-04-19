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
	"github.com/jrhy/sandbox/sql/parse"
)

type Expression struct {
	Parser parse.Parser
	Select *Select `json:",omitempty"`
	Values *Values `json:",omitempty"`
	Errors []error
}

type Schema struct {
	Name    string             `json:",omitempty"`
	Sources map[string]*Schema `json:",omitempty"` // functions/tables
	Columns []SchemaColumn     `json:",omitempty"`
	//GetRowIterator func() RowIterator
}

type SchemaColumn struct {
	Source       string `json:",omitempty"`
	SourceColumn string `json:",omitempty"`
	Name         string `json:",omitempty"`
	DefaultType  string `json:",omitempty"`
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
	Errors      []error            // schema errors
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
	Alias    string   `json:",omitempty"`
	//On Condition
}

type Values struct {
	Rows   []Row  `json:",omitempty"`
	Schema Schema `json:",omitempty"`
	Errors []error
}

type Row []ColumnValue

func (r Row) String() string {
	res := ""
	for i := range r {
		if i > 0 {
			res += " "
		}
		res += r[i].String()
	}
	return res
}

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
	RowFunc   func(Row) ColumnValue
	//Expression OutputExpression //TODO: should maybe SelectExpression+*
}

type Table struct {
	Schema string `json:",omitempty"`
	Table  string `json:",omitempty"`
	Alias  string `json:",omitempty"`
}

var stringValueRE = regexp.MustCompile(`^'([^']*)'`)
var intValueRE = regexp.MustCompile(`^\d+`)
var realValueRE = regexp.MustCompile(`^(((\+|-)?([0-9]+)(\.[0-9]+)?)|((\+|-)?\.?[0-9]+))`)

func values(v *Values) parse.Func {
	addCol := func(cv ColumnValue) {
		n := len(v.Rows) - 1
		v.Rows[n] = append(v.Rows[n], cv)
		if n == 0 {
			v.Schema.Columns = append(v.Schema.Columns,
				SchemaColumn{Name: fmt.Sprintf("column%d", len(v.Schema.Columns)+1)})
		}
	}
	return parse.SeqWS(
		parse.CI("values"),
		parse.Delimited(parse.SeqWS(
			parse.Exact("(").Action(func() {
				v.Rows = append(v.Rows, nil)
			}),
			parse.Delimited(
				parse.SeqWS(
					func(e *parse.Parser) bool {
						var cv ColumnValue
						return e.Match(ColumnValueParser(&cv).Action(func() {
							addCol(cv)
						}))
					}),
				parse.Exact(","),
			),
			parse.Exact(")").Action(func() {
				if len(v.Rows[len(v.Rows)-1]) != len(v.Schema.Columns) {
					v.Errors = append(v.Errors, errors.New("rows with differing column quantity"))
				}
			}),
		),
			parse.Exact(",")),
	)
}
func ColumnValueParser(cv *ColumnValue) parse.Func {
	return parse.OneOf(
		parse.RE(stringValueRE, func(s []string) bool {
			*cv = colval.Text(s[1])
			return true
		}),
		parse.RE(intValueRE, func(s []string) bool {
			i, err := strconv.ParseInt(s[0], 0, 64)
			if err != nil {
				return false
			}
			*cv = colval.Int(i)
			return true
		}),
		parse.RE(realValueRE, func(s []string) bool {
			f, err := strconv.ParseFloat(s[0], 64)
			if err != nil {
				return false
			}
			*cv = colval.Real(f)
			return true
		}),
		parse.CI("null").Action(func() {
			*cv = colval.Null{}
		}),
	)
}

func (b *Expression) Parse() bool {
	var s Select
	var v Values
	return b.Parser.Match(parse.SeqWS(
		parse.OneOf(
			parse.SeqWS(parse.Optional(parse.SeqWS(parse.CI("with"), with(&s.With))),
				ParseSelect(&s)),
			values(&v),
		),
		parse.Optional(parse.Exact(";")),
	).Action(func() {
		b.Select = &s
		b.Values = &v
		b.Errors = append(b.Errors, s.Errors...)
		b.Errors = append(b.Errors, v.Errors...)
	}))
}

func ParseSelect(s *Select) parse.Func {
	return parse.SeqWS(
		parse.CI("select"),
		outputExpressions(&s.Expressions),
		parse.Optional(parse.SeqWS(
			parse.CI("from"),
			fromItems(s),
		)),
		parse.Optional(where(&s.Where)),
		parse.Optional(setOp(&s.SetOp)).Action(func() {
			var schema int
			uniqName := func() string {
				for {
					name := fmt.Sprintf("schema%d", schema)
					if _, ok := s.Schema.Sources[name]; !ok {
						return name
					}
					schema++
				}
			}
			if s.Schema.Sources == nil {
				s.Schema.Sources = make(map[string]*Schema)
			}
			for i := range s.With {
				w := &s.With[i]
				name := w.Name
				if name == "" {
					name = uniqName()
				}
				if _, used := s.Schema.Sources[w.Name]; used {
					s.Errors = append(s.Errors, fmt.Errorf("with %s: appears multiple times", w.Name))
				}
				s.Schema.Sources[name] = &w.Schema
			}
			for i := range s.Values {
				v := &s.Values[i]
				name := uniqName()
				s.Schema.Sources[name] = &v.Schema
			}
			for i := range s.Tables {
				t := &s.Tables[i]
				incomingName := t.Table
				schema, found := s.Schema.Sources[incomingName]
				if !found {
					s.Errors = append(s.Errors, fmt.Errorf("from %s: not found", incomingName))
				} else {
					if t.Alias != "" {
						if _, used := s.Schema.Sources[t.Alias]; used {
							s.Errors = append(s.Errors, fmt.Errorf("from %s: appears multiple times", t.Alias))
						}
						delete(s.Schema.Sources, incomingName)
						s.Schema.Sources[t.Alias] = schema
					}
				}
			}
			if s.Join != nil {
				schema, found := s.Schema.Sources[s.Join.Right]
				if !found {
					s.Errors = append(s.Errors, fmt.Errorf("join %s: not found", s.Join.Right))
				} else {
					if s.Join.Alias != "" {
						if _, used := s.Schema.Sources[s.Join.Alias]; used {
							s.Errors = append(s.Errors, fmt.Errorf("join %s: appears multiple times", s.Join.Alias))
						}
						delete(s.Schema.Sources, s.Join.Right)
						s.Schema.Sources[s.Join.Alias] = schema
					}
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
						var name string
						var found bool
						{
							name = t.Alias
							if name == "" {
								name = t.Table
							}
							schema, found = s.Schema.Sources[name]
							if !found {
								panic("how to handle fromItem resolution failure")
							}
						}
						for _, c := range schema.Columns {
							c := c
							c.Source = name
							c.SourceColumn = c.Name
							s.Schema.Columns = append(s.Schema.Columns, c)
						}
					}
				} else {
					name = o.Expression.Column.Term
					var col *SchemaColumn
					var rs *Schema
					err := s.Schema.resolveColumnRef(name, &rs, &col)
					if err != nil {
						s.Errors = append(s.Errors, fmt.Errorf("select %s: %w", name, err))
					} else if s.Schema.findColumn(col.Name) != nil {
						s.Errors = append(s.Errors, fmt.Errorf("select %s: already selected", name))
					} else {
						name := col.Name
						if o.Alias != "" {
							name = o.Alias
						}
						col := SchemaColumn{
							Source:       rs.Name,
							SourceColumn: col.Name,
							Name:         name,
						}
						s.Schema.Columns = append(s.Schema.Columns, col)
					}
				}
			}
		}))
}

func (s *Schema) resolveColumnRef(name string, schema **Schema, columnRes **SchemaColumn) error {
	var resolvedSchema *Schema
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		return fmt.Errorf("nested schema reference unimplemented: %s", name)
	}
	var column *SchemaColumn
	if len(parts) == 2 {
		var found bool
		s, found = s.Sources[parts[0]]
		if !found {
			return fmt.Errorf("schema not found: %s", name)
		}
		column = s.findColumn(parts[1])
	} else {
		resolvedSchema, column = s.resolveUnqualifiedColumnReference(parts[0])
		if resolvedSchema != nil {
			s = resolvedSchema
		}
	}
	if column == nil {
		return fmt.Errorf("not found: column %s, in %s", name, s.Name)
	}
	if schema != nil {
		*schema = s
	}
	if columnRes != nil {
		*columnRes = column
	}
	return nil
}

func (s *Schema) findColumn(name string) *SchemaColumn {
	for _, c := range s.Columns {
		if strings.ToLower(c.Name) == strings.ToLower(name) {
			return &c
		}
	}
	return nil
}
func (s *Schema) FindColumnIndex(name string) *int {
	for i, c := range s.Columns {
		if strings.ToLower(c.Name) == strings.ToLower(name) {
			return &i
		}
	}
	return nil
}

func (s *Schema) resolveUnqualifiedColumnReference(name string) (*Schema, *SchemaColumn) {
	for _, c := range s.Sources {
		if res := c.findColumn(name); res != nil {
			return c, res
		}
	}
	return nil, nil
}

type Condition struct {
	Left  string `json:",omitempty"` // TODO not just columns but functions
	Right string `json:",omitempty"`
	Op    string `json:",omitempty"`
}

func where(conditions *[]Condition) parse.Func {
	return func(b *parse.Parser) bool {
		var c *Condition
		return b.Match(parse.SeqWS(
			parse.CI("where"),
			parse.Delimited(parse.SeqWS(
				condition(&c).Action(func() {
					*conditions = append(*conditions, *c)
				})),
				parse.CI("and")),
		))
	}
}
func condition(c **Condition) parse.Func {
	return func(b *parse.Parser) bool {
		var left, right, op string
		return b.Match(parse.OneOf(
			// TODO: functions, fix sqlFunc, refactor w/SelectExpression
			parse.SeqWS(name(&left), binaryOp(&op), name(&right)).Action(func() {
				*c = &Condition{Left: left, Right: right, Op: op}
			}),
		))
	}
}
func binaryOp(res *string) parse.Func {
	var Exact = func(s string, res *string) parse.Func {
		return parse.Exact(s).Action(func() { *res = s })
	}
	return parse.OneOf(
		Exact("==", res),
		Exact("=", res),
		Exact("!=", res),
		Exact("<>", res),
	)
}

func join(j **Join) parse.Func {
	return func(b *parse.Parser) bool {
		var joinType JoinType = LeftJoin
		var using, tableName string
		return b.Match(parse.SeqWS(
			parse.Optional(parse.OneOf(
				parse.CI("outer").Action(func() { joinType = OuterJoin }),
				parse.CI("inner").Action(func() { joinType = InnerJoin }),
				parse.CI("left").Action(func() { joinType = LeftJoin }))),
			parse.CI("join"),
			name(&tableName),
			parse.OneOf(
				parse.SeqWS(parse.CI("using"), parse.Exact("("), name(&using), parse.Exact(")")),
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

func setOp(s **SetOp) parse.Func {
	return func(b *parse.Parser) bool {
		var operation SetOpType
		var all bool
		var right Select
		return b.Match(parse.SeqWS(
			parse.OneOf(
				parse.CI("union").Action(func() { operation = Union }),
				parse.CI("intersect").Action(func() { operation = Intersect }),
				parse.CI("except").Action(func() { operation = Except })),
			parse.Optional(parse.CI("all").Action(func() { all = true })),
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
	Name   string `json:",omitempty"`
	Schema Schema
	Values *Values `json:",omitempty"`
	Select *Select `json:",omitempty"`
	Errors []error
}

func with(with *[]With) parse.Func {
	return parse.Delimited(func(e *parse.Parser) bool {
		var w With
		var v Values
		return e.Match(parse.SeqWS(
			parse.RE(sqlNameRE, func(s []string) bool {
				w.Name = s[0]
				return true
			}),
			parse.Optional(parse.SeqWS(
				parse.Exact("("),
				parse.Delimited(
					parse.SeqWS(parse.RE(sqlNameRE, func(s []string) bool {
						w.Schema.Columns = append(w.Schema.Columns, SchemaColumn{Name: s[0]})
						return true
					})),
					parse.Exact(","),
				),
				parse.Exact(")"))),
			parse.CI("as"),
			//ParseSelect(&w.Select),
			parse.Exact("("),
			values(&v),
			parse.Exact(")"),
			parse.Exact("").Action(func() {
				if w.Schema.Columns == nil {
					w.Schema.Columns = v.Schema.Columns
				}
				if len(v.Schema.Columns) != len(v.Schema.Columns) {
					w.Errors = append(w.Errors, errors.New("values mismatch with schema"))
				}
				w.Errors = append(w.Errors, v.Errors...)
				w.Values = &v
				w.Schema.Name = w.Name
				*with = append(*with, w)
			}),
		))
	},
		parse.Exact(","))
}

func outputExpressions(expressions *[]OutputExpression) parse.Func {
	return func(b *parse.Parser) bool {
		var e OutputExpression
		return b.Match(parse.Delimited(
			parse.SeqWS(outputExpression(&e).Action(func() {
				*expressions = append(*expressions, e)
			})),
			parse.Exact(",")))
	}
}
func outputExpression(e *OutputExpression) parse.Func {
	return func(b *parse.Parser) bool {
		var as string
		var f Func
		return b.Match(parse.OneOf(
			parse.SeqWS(sqlFunc(&f), parse.Optional(As(&as))).Action(func() {
				*e = OutputExpression{Expression: SelectExpression{Func: &f}, Alias: as}
			}),
			nameAs(func(name, as string) {
				*e = OutputExpression{Expression: SelectExpression{Column: &Column{Term: name}}, Alias: as}
			}),
			parse.Exact("*").Action(func() {
				*e = OutputExpression{Expression: SelectExpression{Column: &Column{All: true}}}
			}),
		))
	}
}

func sqlFunc(f *Func) parse.Func {
	return func(b *parse.Parser) bool {
		//	var name string
		//	var e OutputExpression
		return b.Match(parse.OneOf(
			parse.SeqWS(parse.CI("current_timestamp")).Action(func() {
				*f = Func{Name: "current_timestamp",
					RowFunc: func(_ Row) ColumnValue {
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

func fromItems(s *Select) parse.Func {
	return parse.SeqWS(
		parse.Delimited(parse.OneOf(
			nameAs(func(name, as string) {
				s.Tables = append(s.Tables, Table{Table: name, Alias: as})
			}),
			func(e *parse.Parser) bool {
				var v Values
				return e.Match(parse.SeqWS(
					parse.Exact("("),
					values(&v),
					parse.Exact(")").Action(func() {
						s.Values = append(s.Values, v)
						s.Schema.Columns = append(s.Schema.Columns, v.Schema.Columns...)
					})))
			},
		), parse.Exact(",")),
		parse.Optional(join(&s.Join)))
}

func name(res *string) parse.Func {
	return func(b *parse.Parser) bool {
		var name string
		return b.Match(parse.SeqWS(
			sqlName(&name), // TODO: '.' should be broken out into separate schema
		).Action(func() { *res = name }))
	}
}

func nameAs(cb func(name, as string)) parse.Func {
	return func(b *parse.Parser) bool {
		var name, as string
		return b.Match(parse.SeqWS(
			sqlName(&name),
			parse.Optional(As(&as)),
		).Action(func() { cb(name, as) }))
	}
}

var sqlNameRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z_0-9-\.]*`)

func sqlName(res *string) parse.Func {
	return parse.RE(sqlNameRE, func(s []string) bool {
		*res = s[0]
		return true
	})
}

func As(as *string) parse.Func {
	return parse.SeqWS(
		parse.CI("as"),
		parse.RE(sqlNameRE, func(s []string) bool {
			*as = s[0]
			return true
		}))
}

func Parse(s string) (*Expression, error) {
	e := &Expression{Parser: parse.Parser{Remaining: s, LastReject: s}}
	if !e.Parse() {
		if e.Parser.Remaining != "" {
			return nil, errors.New("parse error at " + e.Parser.Remaining)
		}
		if len(e.Errors) > 0 {
			// TODO: errutil.Append et al
			return nil, e.Errors[0]
		}
		return nil, errors.New("parse error")
	}

	return e, nil
}

func mustJSON(i interface{}) string {
	var b []byte
	var err error
	b, err = json.Marshal(i)
	if err != nil {
		panic(err)
	}
	if len(b) > 60 {
		b, err = json.MarshalIndent(i, " ", " ")
		if err != nil {
			panic(err)
		}
	}
	return string(b)
}
