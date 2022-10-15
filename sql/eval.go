package sql

import (
	"errors"
	"fmt"

	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/types"
)

func Eval(e *types.Expression, sources map[string]types.Source, cb func(*types.Row) error) error {
	switch {
	case e.Create != nil:
		return errors.New("create must be handled by driver")
	case e.Insert != nil:
		return errors.New("insert must be handled by driver")
		// TODO since "select" and "values" are both included, maybe this should be called "query"
	case e.Select != nil:
		if len(e.Select.OrderBy) > 0 {
			return errors.New("select: ORDER BY must be handled by driver")
		}
		// TODO change this interface to use the RowIteratorForSelect, avoid callback+channels
		return EvalSelect(e.Select, sources, cb)
	}
	return errors.New("unimplemented SQL")
}
func EvalSelect(e *types.Select, sources map[string]types.Source, cb func(*types.Row) error) error {
	i := NewRowIteratorForSelect(e, sources)
	for {
		row, err := i.Next()
		if err != nil {
			return err
		}
		if row == nil {
			break
		}
		if err := cb(row); err != nil {
			return err
		}
	}
	return nil
}

func RowToGo(row types.Row) []interface{} {
	res := make([]interface{}, len(row))
	for i := range row {
		res[i] = colval.ToGo(row[i])
	}
	return res
}
func RowsToGo(rows []types.Row) [][]interface{} {
	res := make([][]interface{}, len(rows))
	for i, row := range rows {
		res[i] = RowToGo(row)
	}
	return res
}
func evaluateConditions(r *RowIteratorForSelect) (bool, error) {
	for i, j := range r.s.Join {
		if len(j.Using) > 0 {
			return false, errors.New("USING not implemented")
		}
		if j.On == nil {
			r.joined[i] = true
			continue
		}
		if res, err := evaluateConditionForRow(j.On, &r.s.Schema, &r.currentRow); !res || err != nil {
			return res, err
		}
		r.joined[i] = true
	}
	if r.s.Where != nil {
		if res, err := evaluateConditionForRow(r.s.Where, &r.s.Schema, &r.currentRow); !res || err != nil {
			return res, err
		}
	}
	return true, nil
}

func evaluateConditionForRow(condition *types.Evaluator, schema *types.Schema, currentRow *map[string]types.Row) (bool, error) {
	inputs := make(map[string]colval.ColumnValue)
	for name := range condition.Inputs {
		var col *types.SchemaColumn
		var rs *types.Schema

		err := ResolveColumnRef(schema, name, &rs, &col)
		if err != nil {
			// XXX TODO this validation should be done in where()
			fmt.Printf("evaluateConditions: %v\n", err)
			return false, err
		}
		sourceRow := (*currentRow)[rs.Name]
		sourceColIndex := FindColumnIndex(rs, col.Name)
		inputs[name] = sourceRow[*sourceColIndex]
	}
	res := condition.Func(inputs).ToBool()
	if res == nil {
		fmt.Printf("condition evaluated to null\n")
	} else {
		fmt.Printf("condition evaluated to %v\n", *res)
	}
	return res != nil && *res, nil
}

func applyOutputExpressions(e *types.Select, fromItems *map[string]types.Row, r *types.Row) {
	var output types.Row = make(types.Row, len(e.Schema.Columns))
	fmt.Printf("AOE going through %d schema columns\n", len(e.Schema.Columns))
	for i, c := range e.Schema.Columns {
		if c.Source == "" {
			fmt.Printf("skipping column %d w/blank source\n", i)
			continue
		}
		fmt.Printf("source is %s\n", c.Source)
		// XXX TODO this wrecks aliasing. Need to make it easier to think what's a "source", "input", or "output", or just in scope of the select.
		sourceSchema := e.Schema.Sources[c.Source]
		if sourceSchema == nil {
			panic(fmt.Errorf("no schema found named '%s' in '%s'", c.Source, e.Schema.Name))
		}
		fmt.Printf("sourceSchema %T %p %+v name=%s\n", sourceSchema, sourceSchema, sourceSchema, c.Source)
		fmt.Printf("fromItems: %+v\n", (*fromItems))
		sourceRow := (*fromItems)[c.Source]
		fmt.Printf("sourceRow %T %p %+v len=%d\n", sourceRow, sourceRow, sourceRow, len(sourceRow))
		fmt.Printf("sourceSchema %s, finding column %s\n", sourceSchema.Name, c.SourceColumn)
		sourceColIndex := FindColumnIndex(sourceSchema, c.SourceColumn)
		if sourceColIndex == nil {
			panic("sourceColIndex nil, aliases not implemented?")
		}
		fmt.Printf("sourceRow: %+v\n", sourceRow)
		fmt.Printf("sourceColIndex %d\n", *sourceColIndex)
		fmt.Printf("assigning output %d %v\n", i, sourceRow[*sourceColIndex])
		output[i] = sourceRow[*sourceColIndex]
	}
	for _, o := range e.Expressions {
		if o.Expression.Func == nil && o.Expression.Column == nil {
			panic("non-Column unimpl")
		}
		// TODO: functions should be evaluated in dependency order
		if o.Expression.Func != nil {
			for i := range e.Schema.Columns {
				if output[i] == nil {
					output[i] = o.Expression.Func.RowFunc(output)
					break
				}
				if i == len(e.Schema.Columns) {
					panic("could not find column for function result")
				}
			}
		}
	}
	fmt.Printf("output: %+v\n", output)
	*r = output
}

/*
	for _, o := range e.Expressions {
		if o.Expression.Func != nil {
			return errors.New("unimpl OutputExpression.Func")
		}
		if o.Expression.Column == nil {
			return errors.New("TODO non-column output expression")
		}
		if !o.Expression.Column.All {
			return errors.New("TODO blah blah blah")
		}
		col := o.Expression.Column
		var cv ColumnValue
		switch strings.ToLower(col.Term) {
		case "current_timestamp": // TODO make part of SelectExpression.Func
			cv.Text = time.Now().UTC().Format("2006-01-02 15:04:05")
		default:
			return fmt.Errorf("unimpl resolve: %s", col.Term)
		}
		// TODO column name
		r = append(r, cv)
	}
	return cb(&r)*/

type RowIteratorForSelect struct {
	s            *types.Select
	done         bool
	sourceToJoin []*int
	joined       []bool
	sources      []*types.Source
	iterators    []types.RowIterator
	dependencies map[string]types.Source
	currentRow   map[string]types.Row
}

func NewRowIteratorForSelect(s *types.Select, sources map[string]types.Source) *RowIteratorForSelect {
	return &RowIteratorForSelect{
		s:            s,
		dependencies: sources,
	}
}

var nestingLevel = 0

func (r *RowIteratorForSelect) Next() (*types.Row, error) {
	nestingLevel++
	defer func() { nestingLevel-- }()
	nestingLevel := nestingLevel
	if r.done {
		return nil, nil
	}
	if len(r.sources) == 0 {
		r.initSources()
		if err := r.initCurrentRow(); err != nil {
			return nil, err
		}
		fmt.Printf("at start of RowIteratorForSelect rlevel=%d, sourceToJoin is: %+v\n", nestingLevel, r.sourceToJoin)
	}
	i := len(r.sources) - 1
	for i >= 0 {
		sourceRow, err := r.iterators[i].Next()
		fmt.Printf("rlevel=%d r=%p sourceRow[%d] TOL %+v\n", nestingLevel, r, i, sourceRow)
		if err != nil {
			return nil, err
		}
		if sourceRow == nil {
			fmt.Printf("rlevel=%d r=%p sourceRow[%d] is null\n", nestingLevel, r, i)
			fmt.Printf("rlevel=%d i=%d r.sourceToJoin[%d]=%v ", nestingLevel, i, i, r.sourceToJoin[i])
			if r.sourceToJoin[i] != nil {
				fmt.Printf("(%d) joined=%v joinType=%v\n",
					*r.sourceToJoin[i],
					r.joined[*r.sourceToJoin[i]],
					r.s.Join[*(r.sourceToJoin[i])].JoinType,
				)
			} else {
				fmt.Printf("\n")
			}
			if r.sourceToJoin[i] != nil && !r.joined[*r.sourceToJoin[i]] &&
				r.s.Join[*(r.sourceToJoin[i])].JoinType == types.LeftJoin {
				// case types.OuterJoin:
				// return nil, errors.New("outer join not yet implemented")
				// case types.RightJoin:
				// return nil, errors.New("right join not yet implemented")
				// nullify and emit last right row
				fmt.Printf("rlevel=%d r=%p evaluating conditions for row:\n\t%+v\n", nestingLevel, r, r.currentRow)
				r.currentRow[r.sources[i].Schema().Name] = make([]colval.ColumnValue, len(r.sources[i].Schema().Columns))
				for j := range r.sources[i].Schema().Columns {
					r.currentRow[r.sources[i].Schema().Name][j] = colval.Null{}
				}
				fmt.Printf("rlevel=%d r=%p evaluating conditions for row:\n\t%+v\n", nestingLevel, r, r.currentRow)
				if r.s.Where != nil {
					if res, err := evaluateConditionForRow(r.s.Where, &r.s.Schema, &r.currentRow); !res || err != nil {
						if err != nil {
							return nil, err
						}

						i = len(r.sources) - 1
						continue
					}
				}
				fmt.Printf("rlevel=%d r=%p sourceRow[%d]is null for right side of left join, so emitting null row for source=%s\n", nestingLevel, r, i, r.sources[i].Schema().Name)
				var res *types.Row = &types.Row{}
				applyOutputExpressions(r.s, &r.currentRow, res)
				fmt.Printf("rlevel=%d r=%p sourceRow[%d] done applying outputExpressions, resulting row:\n\t%+v\n", nestingLevel, r, i, res)
				r.joined[*r.sourceToJoin[i]] = true
				return res, nil
			} else {
				r.iterators[i] = r.sources[i].RowIterator()
				if r.sourceToJoin[i] != nil &&
					r.s.Join[*(r.sourceToJoin[i])].JoinType == types.LeftJoin {
					r.joined[*r.sourceToJoin[i]] = false
				}
				sourceRow, err = r.iterators[i].Next()
				if err != nil {
					return nil, err
				}
				if sourceRow == nil {
					break
				}
				r.currentRow[r.sources[i].Schema().Name] = *sourceRow
				fmt.Printf("rlevel=%d r=%p sourceRow[%d]is null w/o left join, so advancing i--\n", nestingLevel, r, i)
				i--
				continue
			}
		} else {
			if r.sources[i].Schema().Name == "" {
				panic("empty schema name!")
			}
			fmt.Printf("rlevel=%d r=%p sourceRow[%d] assigning currentRow[%s]\n", nestingLevel, r, i, r.sources[i].Schema().Name)
			r.currentRow[r.sources[i].Schema().Name] = *sourceRow
		}
		fmt.Printf("rlevel=%d r=%p evaluating conditions for row:\n\t%+v\n", nestingLevel, r, r.currentRow)
		if res, err := evaluateConditions(r); !res || err != nil {
			if err != nil {
				return nil, err
			}
			fmt.Printf("rlevel=%d r=%p sourceRow[%d] conditions false\n", nestingLevel, r, i)
			i = len(r.sources) - 1
			continue
		}
		var res *types.Row = &types.Row{}
		applyOutputExpressions(r.s, &r.currentRow, res)
		fmt.Printf("rlevel=%d r=%p sourceRow[%d] done applying outputExpressions, resulting row:\n\t%+v\n", nestingLevel, r, i, res)
		return res, nil
	}
	r.done = true
	fmt.Printf("rlevel=%d r=%p sourceRow[%d] done\n", nestingLevel, r, i)
	return nil, nil
}
func (r *RowIteratorForSelect) Schema() *types.Schema {
	return &r.s.Schema
}
func (r *RowIteratorForSelect) initSources() {
	i := 0
	for {
		source := sourceForFromItem(r.s, i, r.dependencies)
		if source == nil {
			break
		}
		r.sources = append(r.sources, source)
		i++
	}

	if i == 0 {
		// if no sources are used, a single empty row is necessary to permit expression-only queries
		schema := types.Schema{Name: "schema0"}
		source := &types.Source{
			Schema: func() types.Schema { return schema },
			RowIterator: func() types.RowIterator {
				return NewRowArrayIterator(&schema,
					[]types.Row{{}})
			},
		}
		r.sources = []*types.Source{source}
		r.sourceToJoin = []*int{nil}
	}
	for i := range r.sources {
		r.iterators = append(r.iterators, r.sources[i].RowIterator())
	}
	r.sourceToJoin = make([]*int, len(r.sources))
	for i := range r.s.Join {
		i := i
		r.sourceToJoin[len(r.sources)-len(r.s.Join)+i] = &i
	}
	r.joined = make([]bool, len(r.s.Join))
}
func (r *RowIteratorForSelect) initCurrentRow() error {
	r.currentRow = make(map[string]types.Row)
	for i := range r.sources {
		if i == len(r.sources)-1 {
			continue
		}
		sourceRow, err := r.iterators[i].Next()
		if err != nil {
			return err
		}
		r.currentRow[r.sources[i].Schema().Name] = *sourceRow
	}
	return nil
}

var _ types.RowIterator = &RowIteratorForSelect{}

func sourceForFromItem(
	r *types.Select, n int, sources map[string]types.Source) *types.Source {
	fmt.Printf("sourceForFromItems %d...\n", n)
	fmt.Printf(" Tables: %+v\n", r.FromItems)
	if r.Values != nil {
		if n == 0 {
			v := r.Values
			fmt.Printf("DBG returning iterator for %+v &v.Schema=%p\n", v, &v.Schema)
			tester := NewRowArrayIterator(&v.Schema, v.Rows)
			for {
				row, err := tester.Next()
				if err != nil {
					panic(err)
				}
				if row != nil {
					fmt.Printf("the row iterator would say %+v\n", *row)
				} else {
					break
				}
			}
			fmt.Printf("returning new iterator, name is %s\n", v.Schema.Name)
			return &types.Source{
				Schema:      func() types.Schema { return v.Schema },
				RowIterator: func() types.RowIterator { return NewRowArrayIterator(&v.Schema, v.Rows) },
			}
		} else {
			n--
		}
	}

	var fromsAndJoins []types.FromItem
	fromsAndJoins = append(fromsAndJoins, r.FromItems...)
	for _, j := range r.Join {
		fromsAndJoins = append(fromsAndJoins, j.Right)
	}

	if n < len(fromsAndJoins) {
		if fromsAndJoins[n].Subquery != nil {
			subquery := fromsAndJoins[n].Subquery
			return &types.Source{
				Schema:      func() types.Schema { return subquery.Schema },
				RowIterator: func() types.RowIterator { return NewRowIteratorForSelect(subquery, sources) },
			}
		}
		name := fromsAndJoins[n].Alias
		if name == "" && fromsAndJoins[n].TableRef != nil {
			name = fromsAndJoins[n].TableRef.Table
		}
		if name == "" {
			panic(fmt.Errorf("no name for fromItem %d", n))
		}
		for _, w := range r.With {
			if w.Name == name {
				fmt.Printf("DBG returning iterator for with %+v\n", w)
				return &types.Source{
					Schema:      func() types.Schema { return w.Schema },
					RowIterator: func() types.RowIterator { return NewRowIteratorForSelect(&w.Select, sources) },
				}
			}
		}
		/* XXX Values,Join should all be generalized to FromItem, interface  */
		source, found := sources[name]
		if !found {
			//TODO add error to return type
			panic(fmt.Errorf("rowIterator not found for fromItem with name=%s", name))
		}
		return &source
	}
	n -= len(fromsAndJoins)
	fmt.Printf("DBG rowIterator %d WE DONE HERE\n", n)
	return nil
}

type RowArrayIterator struct {
	rows   []types.Row
	schema *types.Schema
	i      int
}

func NewRowArrayIterator(schema *types.Schema, rows []types.Row) *RowArrayIterator {
	return &RowArrayIterator{rows: rows, schema: schema}
}

func (r *RowArrayIterator) Next() (*types.Row, error) {
	if r.i == len(r.rows) {
		fmt.Printf("ravi %p row %d (end)\n", r, r.i)
		return nil, nil
	}
	res := types.Row(r.rows[r.i])
	fmt.Printf("ravi %p schema %s row %d %v\n", r, r.schema.Name, r.i, res)
	r.i++
	return &res, nil
}
func (r *RowArrayIterator) Schema() *types.Schema { return r.schema }

/*
func CURRENT_TIMESTAMP() RowIterator {
	return SingleRowIterator{[]ColumnValue{
		{Text: time.Now().UTC().Format("2006-01-02 15:04:05")}},
	}
}
*/
/*
type SingleRowIterator struct {
	Row  Row
	Done bool
}
func (s *SingleRowIterator) Next() (*Row, error) {
	if !done {
		return s.Row, nil
	}
	return nil, nil
}*/
func EvalValues(e *types.Values, cb func(*types.Row) error) error {
	for _, r := range e.Rows {
		err := cb(&r)
		if err != nil {
			return err
		}
	}
	return nil
}
