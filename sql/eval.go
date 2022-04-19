package sql

import (
	"errors"
	"fmt"
)

type Resolver interface {
	Resolve(name string, schemas map[string]*Schema) (RowIterator, error)
}

type RowIterator interface {
	Next() (*Row, error)
	GetSchema() *Schema
}

func Eval(e *Expression, schemas map[string]*Schema, cb func(*Row) error) error {
	switch {
	case e.Select != nil:
		return evalSelect(e.Select, schemas, cb)
	}
	return errors.New("unimplemented SQL")
}
func evalSelect(e *Select, schemas map[string]*Schema, cb func(*Row) error) error {
	var fromItems map[string]Row = make(map[string]Row)
	err := enumerateRows(e, &fromItems, 0, func(fromItems *map[string]Row) error {
		var r *Row = &Row{}
		if false {
			fmt.Printf("cb: want to apply output expressions for these %d fromItems:\n",
				len(*fromItems))
			for schemaName, row := range *fromItems {
				fmt.Printf("\t%s: ", schemaName)
				for _, col := range row {
					fmt.Printf("%v, ", col)
					*r = append(*r, col)
				}
				fmt.Printf("\n")
			}
		} else {
			if !evaluateConditions(e, fromItems, r) {
				return nil
			}
			applyOutputExpressions(e, fromItems, r)
		}
		return cb(r)
	})
	if err != nil {
		panic(err)
	}
	return nil
}

func evaluateConditions(e *Select, fromItems *map[string]Row, r *Row) bool {
	if e.Where == nil {
		return true
	}
	panic("unimpl")
}

func applyOutputExpressions(e *Select, fromItems *map[string]Row, r *Row) {
	var output Row = make(Row, len(e.Schema.Columns))
	fmt.Printf("AOE going through %d schema columns\n", len(e.Schema.Columns))
	for i, c := range e.Schema.Columns {
		if c.Source == "" {
			fmt.Printf("skipping column %d w/blank source\n", i)
			continue
		}
		sourceSchema := e.Schema.Sources[c.Source]
		sourceRow := (*fromItems)[c.Source]
		sourceColIndex := sourceSchema.FindColumnIndex(c.SourceColumn)
		if sourceColIndex == nil {
			panic("sourceColIndex nil, aliases not implemented?")
		}
		output[i] = sourceRow[*sourceColIndex]
		fmt.Printf("assigning output %d %v\n", i, sourceRow[*sourceColIndex])
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

func enumerateRows(e *Select, fromItems *map[string]Row, fromFromItem int, cb func(*map[string]Row) error) error {
	it := rowIteratorForFromItem(e, fromFromItem)
	if it == nil {
		return cb(fromItems)
	}
	fmt.Printf("rowIteratorForFromItem(%d), fromItems:\n", fromFromItem)
	for schemaName, row := range *fromItems {
		fmt.Printf("\trow w/schemaName %s: ", schemaName)
		for i := range row {
			fmt.Printf("%v ", row[i])
		}
		fmt.Printf("\n")
	}

	i := 0
	for {
		fmt.Printf("er %d row %d\n", fromFromItem, i)
		i++
		i, err := it.Next()
		if err != nil {
			return err
		}
		if i == nil {
			return nil
		}
		schemaName := it.GetSchema().Name
		if _, schemaNameUsed := (*fromItems)[schemaName]; schemaNameUsed {
			return fmt.Errorf("schema name is not unique: '%s'", schemaName)
		}
		(*fromItems)[schemaName] = *i
		fmt.Printf("len(fromItems) = %d, name=%s\n", len(*fromItems), schemaName)
		if err = enumerateRows(e, fromItems, fromFromItem+1, cb); err != nil {
			fmt.Printf("badbad %v\n", err)
			return err
		}
		delete(*fromItems, schemaName)
		fmt.Printf("done, len(fromItems) = %d\n", len(*fromItems))
	}
}
func rowIteratorForFromItem(s *Select, n int) RowIterator {
	for _, w := range s.With {
		if n != 0 {
			n--
			continue
		}
		if w.Values != nil {
			return &rowArrayValueIterator{Rows: w.Values.Rows, Schema: &w.Schema}
		} else {
			panic("rowIteratorForFromItem unimpl for subselect")
		}
	}
	/* XXX Values,Join should all be generalized to FromItem, interface  */
	for _, v := range s.Values {
		if n != 0 {
			n--
			continue
		}
		return &rowArrayValueIterator{Rows: v.Rows, Schema: &v.Schema}
	}
	return nil
}

type rowArrayValueIterator struct {
	Rows   []Row
	Schema *Schema
	i      int
}

func (r *rowArrayValueIterator) Next() (*Row, error) {
	fmt.Printf("ravi %p row %d\n", r, r.i)
	if r.i == len(r.Rows) {
		return nil, nil
	}
	res := Row(r.Rows[r.i])
	r.i++
	return &res, nil
}
func (r *rowArrayValueIterator) GetSchema() *Schema { return r.Schema }

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
