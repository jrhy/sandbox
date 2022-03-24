package sql

import (
	"errors"
)

type Schema struct {
	Parent   *Schema            `json:",omitempty"`
	Name     string             `json:",omitempty"`
	Children map[string]*Schema `json:",omitempty"` // functions/tables
	Columns  []SchemaColumn     `json:",omitempty"`
	//GetRowIterator func() RowIterator
}
type SchemaColumn struct {
	Name        string `json:",omitempty"`
	DefaultType string `json:",omitempty"`
}

type Resolver interface {
	Resolve(name string, schemas map[string]*Schema) (RowIterator, error)
}

type RowIterator interface {
	Next() (*Row, error)
}

func Eval(e *Expression, schemas map[string]*Schema, cb func(*Row) error) error {
	switch {
	case e.Select != nil:
		return evalSelect(e.Select, schemas, cb)
	}
	return errors.New("unimplemented SQL")
}
func evalSelect(e *Select, schemas map[string]*Schema, cb func(*Row) error) error {
	r := Row{}
	err := enumerateRows(e, r, 0, func(r *Row) error {
		nr := *r
		applyFromItems(e, &nr)
		return cb(&nr)
	})
	if err != nil {
		panic(err)
	}
	return nil
}
func applyFromItems(e *Select, r *Row) {
	for _, o := range e.Expressions {
		name := o.Alias
		if o.Expression.Func == nil && o.Expression.Column == nil {
			panic("non-Column unimpl")
		}
		if o.Expression.Func != nil {
			*r = append(*r, o.Expression.Func.RowFunc(r))
		} else if o.Expression.Column.All {
		} else if name == "" {
			name = o.Expression.Column.Term
			panic("resolving specific column from schema unimpl")
		}
	}
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
func enumerateRows(e *Select, baseRow Row, fromFromItem int, cb func(*Row) error) error {
	it := rowIteratorForFromItem(e, fromFromItem)
	if it == nil {
		return cb(&baseRow)
	}
	for {
		row := baseRow
		i, err := it.Next()
		if err != nil {
			return err
		}
		if i == nil {
			return nil
		}
		row = append(row, (*i)...)
		if err = enumerateRows(e, row, fromFromItem+1, cb); err != nil {
			return err
		}
	}
}
func rowIteratorForFromItem(s *Select, n int) RowIterator {
	for _, w := range s.With {
		if n != 0 {
			n--
			continue
		}
		if w.Values != nil {
			return &rowArrayValueIterator{Rows: w.Values.Rows}
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
		return &rowArrayValueIterator{Rows: v.Rows}
	}
	if s.Join != nil {
		panic("child schema resolution unimpl for join")
	}
	return nil
}

type rowArrayValueIterator struct {
	Rows []Row
	i    int
}

func (r *rowArrayValueIterator) Next() (*Row, error) {
	if r.i == len(r.Rows) {
		return nil, nil
	}
	res := Row(r.Rows[r.i])
	r.i++
	return &res, nil
}

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
