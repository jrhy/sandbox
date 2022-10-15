package sql

import (
	"errors"
	"fmt"
	"reflect"
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

func values(vp **types.Values) parse.Func {
	return func(b *parse.Parser) bool {
		var v types.Values
		addCol := func(cv colval.ColumnValue) {
			n := len(v.Rows) - 1
			v.Rows[n] = append(v.Rows[n], cv)
			if n == 0 {
				v.Schema.Columns = append(v.Schema.Columns,
					types.SchemaColumn{Name: fmt.Sprintf("column%d", len(v.Schema.Columns)+1)})
			}
		}
		return b.Match(
			parse.SeqWS(
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
					parse.Exact(",")).Action(func() { *vp = &v }),
			))
	}
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

func parseSQLStatement(sources map[string]types.Source, b *types.Expression) bool {
	var c types.Create
	var d types.Drop
	var i types.Insert
	var s types.Select
	return b.Parser.Match(parse.SeqWS(
		parse.OneOf(
			create(&c, sources).Action(func() { b.Create = &c }),
			drop(&d, sources).Action(func() { b.Drop = &d }),
			insert(&i, sources).Action(func() { b.Insert = &i }),
			parse.SeqWS(
				parse.Optional(parse.SeqWS(
					parse.CI("with"),
					// TODO: with should be prefixable to any SQL statement, it just augments/overrides the sources.
					with(&s.With, sources), //
					//Action(func() {
					//b.Select.Schema.Sources
					//})
				)),
				parseSelectStatement(&s, sources)).Action(func() { b.Select = &s }),
		),
		parse.Optional(parse.Exact(";")),
	).Action(func() {
		b.Errors = append(b.Errors, c.Errors...)
		b.Errors = append(b.Errors, d.Errors...)
		b.Errors = append(b.Errors, s.Errors...)
	}))
}

func create(c *types.Create, sources map[string]types.Source) parse.Func {
	return func(b *parse.Parser) bool {
		var name, indexName string
		var expr *types.Evaluator
		//var ifNotExists bool
		return b.Match(parse.SeqWS(
			parse.CI("create"),
			parse.OneOf(
				// index
				parse.SeqWS(
					parse.Optional(parse.CI("unique") /*TODO*/),
					parse.CI("index").Action(func() {
						c.Index = &types.Index{}
						fmt.Printf("XXX INDEX\n")
					}),
					//TODO parse.Optional(parse.SeqWS(parse.CI("if"), parse.CI("not"), parse.CI("exists").Action(func() { ifNotExists = true }))),
					SQLName(&indexName),
					parse.CI("on"),
					SQLName(&name),
					columns(&c.Schema), // TODO permit indexes on expressions, collate, asc/desc
					parse.Optional(parse.SeqWS(
						parse.CI("where"),
						Expression(&expr),
					)).Action(func() {
						c.Schema.Name = indexName
						c.Index.Table = name
						c.Index.Expr = expr
					}),
				),
				// table
				parse.SeqWS(parse.CI("table"),
					//TODO parse.Optional(parse.SeqWS(parse.CI("if"), parse.CI("not"), parse.CI("exists").Action(func() { ifNotExists = true }))),
					SQLName(&name),
					parse.Optional(schema(&c.Schema, &c.Errors).Action(func() {
						c.Schema.Name = name
					})),
					parse.Optional(createQuery(c, &name, sources)),
				),
				// view
				parse.SeqWS(
					parse.Optional(parse.SeqWS(parse.CI("or"), parse.CI("replace").Action(func() { c.View.ReplaceIfExists = true }))),
					parse.CI("view"),
					SQLName(&name),
					parse.Optional(columns(&c.View.Columns)),
					createQuery(c, &name, sources).Action(func() {
						fmt.Printf("HEYA c.Schema.Name is %s\n", c.Schema.Name)
						fmt.Printf("HEYA name is %s\n", name)
					}),
				),
			)))
	}
}

func createQuery(c *types.Create, schemaName *string, sources map[string]types.Source) parse.Func {
	return func(b *parse.Parser) bool {
		var s types.Select
		return b.Match(parse.SeqWS(
			parse.CI("as"),
			parse.Optional(parse.SeqWS(parse.CI("with"), with(&s.With, sources))),
			parseSelectStatement(&s, sources),
		).Action(func() {
			c.Query = &s
			if len(c.Schema.Columns) > 0 && !reflect.DeepEqual(c.Schema.Columns, s.Schema.Columns) {
				// TODO, this was added for table, but may need relaxing for view
				c.Errors = append(c.Errors, errors.New("given columns do not match query"))
			}
			c.Schema = s.Schema
			c.Schema.Name = *schemaName
			fmt.Printf("HEYA schemaName is %s\n", c.Schema.Name)
			c.Errors = append(c.Errors, s.Errors...)
		}))
	}
}

func schema(s *types.Schema, errs *[]error) parse.Func {
	return func(b *parse.Parser) bool {
		var col, coltype, name string
		return b.Match(parse.SeqWS(
			parse.Exact("("),
			parse.Delimited(
				parse.OneOf(
					parse.SeqWS(
						SQLName(&col).
							Action(func() { s.Columns = append(s.Columns, types.SchemaColumn{Name: col}) }),
						parse.Optional(ColumnType(&coltype)).Action(func() {
							s.Columns[len(s.Columns)-1].DefaultType =
								strings.ToLower(coltype)
						}),
						parse.Multiple(
							parse.OneOf(
								parse.SeqWS(parse.CI("primary"), parse.CI("key")).
									Action(func() {
										if len(s.PrimaryKey) > 0 {
											*errs = append(*errs, errors.New("PRIMARY KEY already specified"))
										} else {
											s.PrimaryKey = []string{col}
											*errs = append(*errs, errors.New("PRIMARY KEY is not supported yet"))
										}
									}),
								parse.CI("unique").Action(func() {
									s.Columns[len(s.Columns)-1].Unique = true
									*errs = append(*errs, errors.New("UNIQUE is not supported yet"))
								}),
								parse.SeqWS(parse.CI("not"), parse.CI("null")).Action(func() {
									s.Columns[len(s.Columns)-1].NotNull = true
								}),
							))),
					parse.SeqWS(
						parse.CI("primary"), parse.CI("key"),
						parse.Exact("(").
							Action(func() {
								if len(s.PrimaryKey) > 0 {
									*errs = append(*errs, errors.New("PRIMARY KEY specified multiple times"))
								}
							}),
						parse.Delimited(
							parse.SeqWS(
								SQLName(&col).
									Action(func() {
										s.PrimaryKey = append(s.PrimaryKey, col)
									})),
							parse.Exact(",")),
						parse.Exact(")"))),
				parse.Exact(",")),
			parse.Exact(")")).Action(func() { s.Name = name }))
	}
}

func drop(d *types.Drop, sources map[string]types.Source) parse.Func {
	return func(b *parse.Parser) bool {
		var name string
		return b.Match(parse.SeqWS(
			parse.CI("drop"),
			parse.OneOf(
				parse.CI("index").Action(func() { d.Errors = append(d.Errors, errors.New("DROP INDEX is not implemented")) }),
				parse.CI("table"),
				parse.CI("trigger").Action(func() { d.Errors = append(d.Errors, errors.New("DROP TRIGGER is not implemented")) }),
				parse.CI("view").Action(func() { d.Errors = append(d.Errors, errors.New("DROP VIEW is not implemented")) }),
			),
			parse.Optional(parse.SeqWS(
				parse.CI("if"),
				parse.CI("exists")).Action(func() {
				d.IfExists = true
			})),
			SQLName(&name).Action(func() {
				if _, found := sources[name]; !found {
					d.Errors = append(d.Errors, fmt.Errorf("%s: not found", name))
				}
				d.TableRef = &types.TableRef{
					Table: name,
				}
			}),
		))
	}
}

func insert(i *types.Insert, sources map[string]types.Source) parse.Func {
	return func(b *parse.Parser) bool {
		var name, alias string
		var s types.Select
		return b.Match(parse.SeqWS(
			parse.CI("insert"),
			parse.CI("into"),
			SQLName(&name),
			parse.Optional(parse.SeqWS(
				parse.CI("as"),
				SQLName(&alias))),
			parse.Optional(columns(&i.Schema)),
			parse.SeqWS(parse.Optional(parse.SeqWS(parse.CI("with"), with(&s.With, sources))),
				parseSelectStatement(&s, sources)).Action(func() {
				i.Query = &s
				// XXX this whole pattern is repeated for create and maybe elsewhere, needs testing.
				if len(i.Schema.Columns) > 0 && !reflect.DeepEqual(i.Schema.Columns, s.Schema.Columns) {
					i.Errors = append(i.Errors, errors.New("given columns do not match query"))
				}
				i.Query = &s
				i.Schema = s.Schema
				i.Errors = append(i.Errors, s.Errors...)
			}),
		).Action(func() {
			if alias != "" {
				i.Schema.Name = alias
			} else {
				i.Schema.Name = name
			}
		}))
	}
}

func columns(s *types.Schema) parse.Func {
	var col string
	return parse.SeqWS(parse.Exact("("),
		parse.Delimited(
			parse.SeqWS(SQLName(&col).Action(func() {
				s.Columns = append(s.Columns, types.SchemaColumn{Name: col})
			})),
			parse.Exact(","),
		),
		parse.Exact(")"))
}

func debugHere(s string) parse.Func {
	return func(p *parse.Parser) bool {
		fmt.Printf("DBG debugHere %s, remaining: %s\n", s, p.Remaining)
		return true
	}
}
func parseSelectStatement(s *types.Select, sources map[string]types.Source) parse.Func {
	return parse.OneOf(
		values(&s.Values).Action(func() {
			if s.Values.Schema.Name != "" {
				panic("v.Schema.Name is unexpectedly set")
			}
			s.Values.Schema.Name = "values"
			s.Schema = s.Values.Schema
			for i := range s.Schema.Columns {
				s.Schema.Columns[i].Source = "values"
				s.Schema.Columns[i].SourceColumn = s.Schema.Columns[i].Name
			}
			if s.Schema.Sources == nil {
				s.Schema.Sources = map[string]*types.Schema{}
			}
			s.Schema.Sources["values"] = &s.Values.Schema
		}),
		parse.SeqWS(
			parse.CI("select").Action(func() { fmt.Printf("DBG STARTING SELECT\n") }),
			debugHere("after select"),
			outputExpressions(&s.Expressions).Action(func() { fmt.Printf("DBG expressions: %+v\n", s.Expressions) }),
			debugHere("after outputExpressions"),
			parse.Optional(parse.SeqWS(
				parse.CI("from").Action(func() { fmt.Printf("DBG FROM\n") }),
				fromItems(s, sources),
			)).Action(func() { fmt.Printf("DBG GOT FROM: %+v\n", s.FromItems) }),
			parse.Optional(where(&s.Where)),
			parse.Optional(setOp(&s.SetOp, sources)),
			parse.Optional(orderBy(&s.OrderBy, sources)),
			parse.Optional(limit(&s.Limit, &s.Errors)),
			parse.Optional(offset(&s.Offset, &s.Errors)),
		).Action(func() {
			var schema int
			uniqName := func() string {
				for {
					name := fmt.Sprintf("schema%d", schema)
					if _, ok := s.Schema.Sources[name]; !ok {
						schema++
						return name
					}
					schema++
				}
			}
			if s.Schema.Sources != nil {
				panic("XXX why is s.Schema.Sources already set")
			} else {
				s.Schema.Sources = make(map[string]*types.Schema)
			}
			for name, source := range sources {
				schema := source.Schema()
				s.Schema.Sources[name] = &schema
			}
			for i := range s.With {
				w := &s.With[i]
				if w.Name == "" {
					w.Name = uniqName()
				}
				if _, used := s.Schema.Sources[w.Name]; used {
					s.Errors = append(s.Errors, fmt.Errorf("with %s: appears multiple times", w.Name))
				}
				if w.Schema.Name == "" {
					w.Schema.Name = w.Name
				}
				s.Schema.Sources[w.Name] = &w.Schema
			}
			// XXX TODO this should be generalized w.r.t. subselects, aliases
			if s.Values != nil {
				v := s.Values
				if v.Name == "" {
					v.Name = uniqName()
				}
				if _, used := s.Schema.Sources[v.Name]; used {
					s.Errors = append(s.Errors, fmt.Errorf("values %s: appears multiple times", v.Name))
				}
				if v.Schema.Name == "" {
					v.Schema.Name = v.Name
				}
				v.Schema.Name = v.Name
				s.Schema.Sources[v.Name] = &v.Schema
			}
			var fromsAndJoins []types.FromItem
			fromsAndJoins = append(fromsAndJoins, s.FromItems...)
			for _, j := range s.Join {
				fromsAndJoins = append(fromsAndJoins, j.Right)
			}

			for _, f := range fromsAndJoins {
				var incomingName string
				var schema *types.Schema
				var found bool
				if t := f.TableRef; t != nil {
					incomingName := t.Table
					schema, found = s.Schema.Sources[incomingName]
					if !found {
						fmt.Printf("DBG NO FOUND SOURCE FOR %s\n", incomingName)
						s.Errors = append(s.Errors, fmt.Errorf("from %s: not found", incomingName))
					} else {
						fmt.Printf("DBG FOUND SOURCE FOR %s\n", incomingName)
					}
					if f.Alias != "" {
						fmt.Printf("DBG setting fromItem alias %s->%s\n", incomingName, f.Alias)
						if _, used := s.Schema.Sources[f.Alias]; used {
							s.Errors = append(s.Errors, fmt.Errorf("from %s: appears multiple times", f.Alias))
						}
						delete(s.Schema.Sources, incomingName)
						s.Schema.Sources[f.Alias] = schema
					}

				} else if sub := f.Subquery; sub != nil {
					// incomingName = sub.Schema.Name
					// if incomingName == "" {
					// fmt.Printf("DBG subquery schema name was blank\n")
					// XXX original sub.Schema.Name is bs
					if f.Alias != "" {
						incomingName = f.Alias
					} else {
						incomingName = uniqName()
					}
					// } else {
					// fmt.Printf("DBG subquery schema name was %s\n", incomingName)
					// }
					sub.Schema.Name = incomingName
					s.Schema.Sources[incomingName] = &sub.Schema
				} else {
					panic("unknown fromItem")
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
					fmt.Printf("o.Expression.Column.All:\n")
					for _, t := range fromsAndJoins {
						var schema *types.Schema
						var name string
						var found bool
						if t.Subquery != nil {
							name = t.Subquery.Schema.Name
							schema = &t.Subquery.Schema
						} else {
							name = t.Alias
							t := *t.TableRef
							if name == "" {
								name = t.Table
							}
							schema, found = s.Schema.Sources[name]
							// XXX this is dirty, schema layering should be simpler
							if !found && sources != nil {
								source, found := sources[name]
								if !found {
									fmt.Printf("sources: %+v\n", sources)
									fmt.Printf("no source found for name '%s'\n", name)
									panic("how to handle fromItem resolution failure")
								}
								sp := source.Schema()
								schema = &sp
								// XXX TODO if this Sources map is actually shared, this is too complicated, switch to mast.
								s.Schema.Sources[name] = schema
							}
						}
						fmt.Printf("now that we are looking, the s.Schema columns are: %+v\n", s.Schema.Columns)
						fmt.Printf("now that we are looking, the schema columns are: %+v\n", schema.Columns)
						for _, c := range schema.Columns {
							fmt.Printf("wildcard output includes %s\n", c.Name)
							c := c
							c.Source = name
							c.SourceColumn = c.Name
							s.Schema.Columns = append(s.Schema.Columns, c)
						}
					}
					// XXX terminology: subselect -> subquery
					// XXX TODO this should be generalized w.r.t. subselects, aliases
					if s.Values != nil {
						v := s.Values
						schema := &v.Schema
						if v.Schema.Name == "" {
							fmt.Printf("DBG needs a uniq name for v.Name=%s", v.Name)
							panic(fmt.Errorf("needs a uniq name for v.Name=%s", v.Name))
						} else {
							fmt.Printf("DBG values has a uniq name %s %s\n", v.Name, v.Schema.Name)
						}
						s.Schema.Sources[v.Name] = schema
						for _, c := range schema.Columns {
							c := c
							c.Source = v.Name
							c.SourceColumn = c.Name
							s.Schema.Columns = append(s.Schema.Columns, c)
							fmt.Printf("wildcard output now includes %+v\n", c)
						}
					}
					fmt.Printf("now, output schema is %d columns\n", len(s.Schema.Columns))
					if len(s.Schema.Columns) > 0 {
						fmt.Printf("the first column is %+v\n", s.Schema.Columns[0])
					}

				} else {
					name = o.Expression.Column.Term
					fmt.Printf("o.Expression.Column.Term: %+v\n", name)
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

func limit(res **int64, errors *[]error) parse.Func {
	return parse.SeqWS(parse.CI("limit"), parseExpressionToInt(res, "limit", errors))
}
func offset(res **int64, errors *[]error) parse.Func {
	return parse.SeqWS(parse.CI("offset"), parseExpressionToInt(res, "offset", errors))
}
func parseExpressionToInt(res **int64, name string, errors *[]error) parse.Func {
	var e *types.Evaluator
	return func(b *parse.Parser) bool {
		return b.Match(
			Expression(&e).Action(func() {
				if len(e.Inputs) > 0 {
					fmt.Printf("inputs: %+v\n", e.Inputs)
					*errors = append(*errors, fmt.Errorf("%s: int expression cannot reference", name))
					return
				}
				v := e.Func(nil)
				switch x := v.(type) {
				case colval.Int:
					i := int64(x)
					*res = &i
				default:
					*errors = append(*errors, fmt.Errorf("%s: got %T, expected int expression", name, x))
				}
			}),
		)
	}
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
			fmt.Printf("returning MATCH column %d, c.Name=%s, name=%s\n", i, c.Name, name)
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

func joins(js *[]types.Join, sources map[string]types.Source) parse.Func {
	return parse.Delimited(
		func(b *parse.Parser) bool {
			var j *types.Join
			return b.Match(join(&j, sources).Action(func() {
				fmt.Printf("DBG found a join: %+v\n", *j)
				*js = append(*js, *j)
			}))
		},
		parse.Exact(""))
}

func join(j **types.Join, sources map[string]types.Source) parse.Func {
	return func(b *parse.Parser) bool {
		var joinType types.JoinType = types.InnerJoin
		var colName string
		var using []string
		var joinCondition *types.Evaluator
		var fi types.FromItem
		return b.Match(parse.SeqWS(
			parse.Optional(parse.OneOf(
				parse.CI("outer").Action(func() { joinType = types.OuterJoin }),
				parse.CI("inner"),
				parse.CI("left").Action(func() { joinType = types.LeftJoin }))),
			parse.CI("join"),
			fromItem(&fi, sources),
			parse.OneOf(
				parse.SeqWS(
					parse.CI("using"),
					parse.Exact("("),
					parse.Delimited(
						parse.SeqWS(name(&colName).Action(func() { using = append(using, colName) })),
						parse.Exact(",")),
					parse.Exact(")")),
				parse.SeqWS(
					parse.CI("on"),
					Expression(&joinCondition)),
				parse.Exact(""),
			)).
			// plan:
			// DONE * add a RowIterator for Select, that is stateful and nonrecursive
			// DONE * change Value to Subexpression
			// DONE * change Table to FromItem (TableOrSubexpression)
			// * change With to StatementPrefix as a schema modifier
			// * add bind-parameters
			// * join would be pretty great
			Action(func() {
				*j = &types.Join{
					JoinType: joinType,
					Right:    fi,
					On:       joinCondition,
					Using:    using,
				}
			}))
	}
}

func setOp(s **types.SetOp, sources map[string]types.Source) parse.Func {
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
			parseSelectStatement(&right, sources).Action(func() {
				*s = &types.SetOp{
					Op:    operation,
					All:   all,
					Right: &right,
				}
			})))
	}
}

func with(with *[]types.With, sources map[string]types.Source) parse.Func {
	return parse.Delimited(func(e *parse.Parser) bool {
		var col string
		var w types.With
		return e.Match(parse.SeqWS(
			SQLName(&w.Name),
			parse.Optional(parse.SeqWS(
				parse.Exact("("),
				parse.Delimited(
					parse.SeqWS(SQLName(&col).Action(func() {
						w.Schema.Columns = append(w.Schema.Columns, types.SchemaColumn{Name: col})
					})),
					parse.Exact(","),
				),
				parse.Exact(")"))),
			parse.CI("as"),
			parse.Exact("("),
			parseSelectStatement(&w.Select, sources),
			parse.Exact(")").Action(func() { fmt.Printf("DBG matched subselect...\n") }),
			parse.Exact("").Action(func() {
				if w.Schema.Columns == nil {
					w.Schema.Columns = w.Select.Schema.Columns
				} else if len(w.Schema.Columns) != len(w.Select.Schema.Columns) {
					w.Errors = append(w.Errors, fmt.Errorf("subquery has %d columns but %s has %d",
						len(w.Select.Schema.Columns), w.Name, len(w.Schema.Columns)))
				}
				for i := range w.Schema.Columns {
					w.Schema.Columns[i].Source = w.Select.Schema.Name
					w.Schema.Columns[i].SourceColumn = w.Select.Schema.Columns[i].Name
				}
				if w.Schema.Sources == nil {
					w.Schema.Sources = map[string]*types.Schema{}
				}
				w.Schema.Sources[w.Select.Schema.Name] = &w.Select.Schema
				w.Schema.Name = w.Name
				*with = append(*with, w)
				fmt.Printf("DBG appending %+v\n", w)
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
				fmt.Printf("DBG OE matched func\n")
				*e = types.OutputExpression{Expression: types.SelectExpression{Func: &f}, Alias: as}
			}),
			nameAs(func(name, as string) {
				fmt.Printf("DBG OE matched nameAs name=%s as=%s\n", name, as)
				*e = types.OutputExpression{Expression: types.SelectExpression{Column: &types.Column{Term: name}}, Alias: as}
			}),
			parse.Exact("*").Action(func() {
				fmt.Printf("DBG OE matched *\n")
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

func fromItems(s *types.Select, sources map[string]types.Source) parse.Func {
	return parse.SeqWS(
		parse.Delimited(
			func(e *parse.Parser) bool {
				var fi types.FromItem
				if res := e.Match(fromItem(&fi, sources)); res {
					s.FromItems = append(s.FromItems, fi)
					return true
				}
				return false
			},
			parse.Exact(","),
		),
		parse.Optional(joins(&s.Join, sources)))
}

func fromItem(fi *types.FromItem, sources map[string]types.Source) parse.Func {
	return parse.OneOf(
		nameAs(func(name, as string) {
			fmt.Printf("DBG GOT NAME %s ALIAS=%s\n", name, as)
			*fi = types.FromItem{TableRef: &types.TableRef{Table: name}, Alias: as}
		}),
		func(e *parse.Parser) bool {
			var subquery types.Select
			var as string
			return e.Match(parse.SeqWS(
				parse.Exact("("),
				parseSelectStatement(&subquery, sources),
				parse.Exact(")"),
				parse.Optional(FromItemAs(&as)).
					Action(func() {
						*fi = types.FromItem{Subquery: &subquery, Alias: as}
					})))
		},
	)

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
			parse.Optional(FromItemAs(&as)),
		).Action(func() { cb(name, as) }))
	}
}

var sqlNameRE = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z_0-9-\.]*)|^('(([^']|'')*)'|"([^"]*)")`)

func SQLName(res *string) parse.Func {
	return parse.RE(sqlNameRE, func(s []string) bool {
		if len(s[1]) > 0 {
			*res = s[1]
		} else if len(s[3]) > 0 {
			*res = strings.ReplaceAll(s[3], `''`, `'`)
		} else {
			*res = s[5]
		}
		return true
	})
}

var typeRE = regexp.MustCompile(`^(?i:text|varchar|integer|number|real)`)

func ColumnType(res *string) parse.Func {
	return parse.RE(typeRE, func(s []string) bool {
		*res = s[0]
		return true
	})
}

func As(as *string) parse.Func {
	return parse.SeqWS(
		parse.Optional(parse.CI("as")),
		SQLName(as))
}

func FromItemAs(as *string) parse.Func {
	return parse.SeqWS(
		parse.Exact("").Action(func() { fmt.Printf("DBG starting fromAs") }),
		parse.Optional(parse.CI("as")),
		FromItemAlias(as))
}

var sqlKeywordRE = regexp.MustCompile(`^(?i:select|from|where|join|outer|inner|left|on|using|union|except|group|all|distinct|order|limit|offset)`)
var matchKeyword = parse.RE(sqlKeywordRE, func(_ []string) bool { return true })

func FromItemAlias(res *string) parse.Func {
	return func(p *parse.Parser) bool {
		if matchKeyword(p) {
			return false
		}
		return SQLName(res)(p)
	}
}

func Parse(sources map[string]types.Source, s string) (*types.Expression, error) {
	e := &types.Expression{Parser: parse.Parser{Remaining: s, LastReject: s}}
	if !parseSQLStatement(sources, e) {
		if len(e.Errors) > 0 {
			// TODO: errutil.Append et al
			return nil, e.Errors[0]
		}
		if e.Parser.Remaining != "" {
			return nil, errors.New("parse error at " + e.Parser.Remaining)
		}
		return nil, errors.New("parse error")
	}
	if len(e.Errors) > 0 {
		// TODO: errutil.Append et al
		return nil, e.Errors[0]
	}
	if e.Parser.Remaining != "" {
		return nil, errors.New("parse error at " + e.Parser.Remaining)
	}
	// fmt.Printf("Parsed to: %s\n", mustJSON(e))

	return e, nil
}

// TODO: consider if the constraint on 'insert or replace' or 'insert or ignore' for tables with keys may be an intuitive way to convey how coordinatorless tables will work.
// Relatedly, need ability to set write time, for update idempotence.

func orderBy(op *[]types.OrderBy, sources map[string]types.Source) parse.Func {
	return func(b *parse.Parser) bool {
		var o types.OrderBy
		return b.Match(parse.SeqWS(
			parse.CI("order"),
			parse.CI("by"),
			parse.Delimited(
				parse.SeqWS(Expression(&o.Expression).Action(func() {
					if len(o.Expression.Inputs) == 0 {
						if col, ok := o.Expression.Func(nil).(colval.Int); ok {
							o.OutputColumn = int(col)
							o.Expression = nil
						} else {
							panic("TODO error order by with non-integer column")
						}
					} else {
						panic("TODO error expecting column number as integer")
					}
				}),
					parse.Optional(
						parse.OneOf(
							parse.CI("asc"),
							parse.CI("desc").Action(func() { o.Desc = true }),
							//TODO parse.CI("using"), operator...,
						)),
					parse.Optional(parse.SeqWS(
						parse.CI("nulls"),
						parse.OneOf(
							parse.CI("first").Action(func() { o.NullsFirst = true }),
							parse.CI("last")))),
				).Action(func() {
					*op = append(*op, o)
					o = types.OrderBy{}
				}),
				parse.Exact(","))))
	}
}
