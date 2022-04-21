package sql

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jrhy/sandbox/parse"
	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/types"
)

var StringValueRE = regexp.MustCompile(`^('(([^']|'')*)'|"([^"]*)")`)
var IntValueRE = regexp.MustCompile(`^\d+`)
var RealValueRE = regexp.MustCompile(`^(((\+|-)?([0-9]+)(\.[0-9]+)?)|((\+|-)?\.?[0-9]+))([Ee]\d+)?`)

func values(v *types.Values) parse.Func {
	addCol := func(cv colval.ColumnValue) {
		n := len(v.Rows) - 1
		v.Rows[n] = append(v.Rows[n], cv)
		if n == 0 {
			v.Schema.Columns = append(v.Schema.Columns,
				types.SchemaColumn{Name: fmt.Sprintf("column%d", len(v.Schema.Columns)+1)})
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
						var cv colval.ColumnValue
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

func ColumnValueParser(cv *colval.ColumnValue) parse.Func {
	return parse.OneOf(
		parse.RE(StringValueRE, func(s []string) bool {
			if len(s[2]) > 0 {
				*cv = colval.Text(strings.ReplaceAll(s[2], `''`, `'`))
			} else {
				*cv = colval.Text(s[4])
			}
			return true
		}),
		parse.RE(RealValueRE, func(s []string) bool {
			if !strings.ContainsAny(s[0], ".eE") {
				fmt.Printf("parsing as int...\n")
				i, err := strconv.ParseInt(s[0], 0, 64)
				fmt.Printf("the int: %d, conversion error: %v\n", i, err)
				if err == nil {
					*cv = colval.Int(i)
					return true
				}
				// too big or whatever, try again as real
			}
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

func ParseSQLStatement(b *types.Expression) bool {
	var s types.Select
	var v types.Values
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

func ParseSelect(s *types.Select) parse.Func {
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
				s.Schema.Sources = make(map[string]*types.Schema)
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
				var name string
				if o.Expression.Func == nil && o.Expression.Column == nil {
					panic("non-Column unimpl")
				}
				if o.Expression.Func != nil {
					if o.Alias != "" {
						s.Schema.Columns = append(s.Schema.Columns, types.SchemaColumn{Name: o.Alias})
					} else {
						s.Schema.Columns = append(s.Schema.Columns, types.SchemaColumn{Name: o.Expression.Func.Name})
					}
				} else if o.Expression.Column.All {
					for _, t := range s.Tables {
						var schema *types.Schema
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
					var col *types.SchemaColumn
					var rs *types.Schema
					err := ResolveColumnRef(&s.Schema, name, &rs, &col)
					if err != nil {
						s.Errors = append(s.Errors, fmt.Errorf("select %s: %w", name, err))
					} else if findColumn(&s.Schema, col.Name) != nil {
						s.Errors = append(s.Errors, fmt.Errorf("select %s: already selected", name))
					} else {
						name := col.Name
						if o.Alias != "" {
							name = o.Alias
						}
						col := types.SchemaColumn{
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

func ResolveColumnRef(s *types.Schema, name string, schema **types.Schema, columnRes **types.SchemaColumn) error {
	var resolvedSchema *types.Schema
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		return fmt.Errorf("nested schema reference unimplemented: %s", name)
	}
	var column *types.SchemaColumn
	if len(parts) == 2 {
		var found bool
		s, found = s.Sources[parts[0]]
		if !found {
			return fmt.Errorf("schema not found: %s", name)
		}
		column = findColumn(s, parts[1])
	} else {
		resolvedSchema, column = resolveUnqualifiedColumnReference(s, parts[0])
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

func findColumn(s *types.Schema, name string) *types.SchemaColumn {
	for _, c := range s.Columns {
		if strings.EqualFold(c.Name, name) {
			return &c
		}
	}
	return nil
}
func FindColumnIndex(s *types.Schema, name string) *int {
	for i, c := range s.Columns {
		if strings.EqualFold(c.Name, name) {
			return &i
		}
	}
	return nil
}

func resolveUnqualifiedColumnReference(s *types.Schema, name string) (*types.Schema, *types.SchemaColumn) {
	for _, c := range s.Sources {
		if res := findColumn(c, name); res != nil {
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

func where(evaluator **types.Evaluator) parse.Func {
	return func(b *parse.Parser) bool {
		return b.Match(parse.SeqWS(
			parse.CI("where"),
			Expression(evaluator),
		))
	}
}

func join(j **types.Join) parse.Func {
	return func(b *parse.Parser) bool {
		var joinType types.JoinType = types.LeftJoin
		var using, tableName string
		return b.Match(parse.SeqWS(
			parse.Optional(parse.OneOf(
				parse.CI("outer").Action(func() { joinType = types.OuterJoin }),
				parse.CI("inner").Action(func() { joinType = types.InnerJoin }),
				parse.CI("left").Action(func() { joinType = types.LeftJoin }))),
			parse.CI("join"),
			name(&tableName),
			parse.OneOf(
				parse.SeqWS(parse.CI("using"), parse.Exact("("), name(&using), parse.Exact(")")),
				//SeqWS(CI("on"), condition),
			)).Action(func() {
			*j = &types.Join{
				JoinType: joinType,
				Right:    tableName,
				Using:    using,
			}
		}))
	}
}

func setOp(s **types.SetOp) parse.Func {
	return func(b *parse.Parser) bool {
		var operation types.SetOpType
		var all bool
		var right types.Select
		return b.Match(parse.SeqWS(
			parse.OneOf(
				parse.CI("union").Action(func() { operation = types.Union }),
				parse.CI("intersect").Action(func() { operation = types.Intersect }),
				parse.CI("except").Action(func() { operation = types.Except })),
			parse.Optional(parse.CI("all").Action(func() { all = true })),
			ParseSelect(&right).Action(func() {
				*s = &types.SetOp{
					Op:    operation,
					All:   all,
					Right: &right,
				}
			})))
	}
}

func with(with *[]types.With) parse.Func {
	return parse.Delimited(func(e *parse.Parser) bool {
		var w types.With
		var v types.Values
		return e.Match(parse.SeqWS(
			parse.RE(sqlNameRE, func(s []string) bool {
				w.Name = s[0]
				return true
			}),
			parse.Optional(parse.SeqWS(
				parse.Exact("("),
				parse.Delimited(
					parse.SeqWS(parse.RE(sqlNameRE, func(s []string) bool {
						w.Schema.Columns = append(w.Schema.Columns, types.SchemaColumn{Name: s[0]})
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
				if len(v.Schema.Columns) != len(w.Schema.Columns) {
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

func outputExpressions(expressions *[]types.OutputExpression) parse.Func {
	return func(b *parse.Parser) bool {
		var e types.OutputExpression
		return b.Match(parse.Delimited(
			parse.SeqWS(outputExpression(&e).Action(func() {
				*expressions = append(*expressions, e)
			})),
			parse.Exact(",")))
	}
}
func outputExpression(e *types.OutputExpression) parse.Func {
	return func(b *parse.Parser) bool {
		var as string
		var f types.Func
		return b.Match(parse.OneOf(
			parse.SeqWS(sqlFunc(&f), parse.Optional(As(&as))).Action(func() {
				*e = types.OutputExpression{Expression: types.SelectExpression{Func: &f}, Alias: as}
			}),
			nameAs(func(name, as string) {
				*e = types.OutputExpression{Expression: types.SelectExpression{Column: &types.Column{Term: name}}, Alias: as}
			}),
			parse.Exact("*").Action(func() {
				*e = types.OutputExpression{Expression: types.SelectExpression{Column: &types.Column{All: true}}}
			}),
		))
	}
}

func sqlFunc(f *types.Func) parse.Func {
	return func(b *parse.Parser) bool {
		//	var name string
		//	var e OutputExpression
		return b.Match(parse.OneOf(
			parse.SeqWS(parse.CI("current_timestamp")).Action(func() {
				*f = types.Func{Name: "current_timestamp",
					RowFunc: func(_ types.Row) colval.ColumnValue {
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

func fromItems(s *types.Select) parse.Func {
	return parse.SeqWS(
		parse.Delimited(parse.OneOf(
			nameAs(func(name, as string) {
				s.Tables = append(s.Tables, types.Table{Table: name, Alias: as})
			}),
			func(e *parse.Parser) bool {
				var v types.Values
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
			SQLName(&name), // TODO: '.' should be broken out into separate schema
		).Action(func() { *res = name }))
	}
}

func nameAs(cb func(name, as string)) parse.Func {
	return func(b *parse.Parser) bool {
		var name, as string
		return b.Match(parse.SeqWS(
			SQLName(&name),
			parse.Optional(As(&as)),
		).Action(func() { cb(name, as) }))
	}
}

var sqlNameRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z_0-9-\.]*`)

func SQLName(res *string) parse.Func {
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

func Parse(s string) (*types.Expression, error) {
	e := &types.Expression{Parser: parse.Parser{Remaining: s, LastReject: s}}
	if !ParseSQLStatement(e) {
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
