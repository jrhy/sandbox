package db

import (
	"context"
	"fmt"
	"sort"

	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/driver/interfaces"
	"github.com/jrhy/sandbox/sql/types"
)

type Mem struct {
	index map[string]*MemIndex
	table map[string]*MemTable
	view  map[string]*MemView
}

type MemIndex struct {
	schema                *types.Schema
	expr                  *types.Evaluator
	table                 *MemTable
	rows                  []types.Row
	exprColumnNameToIndex map[string]int
	indexToTableColumn    []int
	inputs                map[string]colval.ColumnValue
}

type MemTable struct {
	schema  *types.Schema
	rows    []types.Row
	indices []*MemIndex
}

type MemView struct {
	schema *types.Schema
	query  string
}

var _ interfaces.DB = &Mem{}

func NewMem() *Mem {
	return &Mem{
		index: make(map[string]*MemIndex),
		table: make(map[string]*MemTable),
		view:  make(map[string]*MemView),
	}
}

func (d *Mem) GetTable(ctx context.Context, name string) (interfaces.Table, bool, error) {
	t, found := d.table[name]
	return t, found, nil
}

func (d *Mem) DropTable(ctx context.Context, name string) error {
	delete(d.table, name)
	return nil
}

func (d *Mem) CreateTable(ctx context.Context, name string, schema *types.Schema) (interfaces.Table, error) {
	_, found := d.table[name]
	if found {
		panic(fmt.Errorf("table creation not synchronized: %s", name))
	}
	t := &MemTable{
		schema: schema,
	}
	d.table[name] = t
	return t, nil
}

func (t *MemTable) Insert(ctx context.Context, rows []types.Row) error {
	t.rows = append(t.rows, rows...)
	for _, mi := range t.indices {
		needSort := false
		for _, r := range rows {
			if mi.updateIndexForSourceRow(&r) {
				needSort = true
			}
		}
		if needSort {
			sort.Sort(&rowArraySort{mi.rows})
		}
	}
	return nil
}

func (t *MemTable) Source() types.Source {
	return types.Source{
		RowIterator: func() types.RowIterator {
			return sql.NewRowArrayIterator(t.schema, t.rows)
		},
		// XXX TODO no reason not to just have a pointer to the schema...
		Schema: func() types.Schema {
			return *t.schema
		},
	}
}

func (d *Mem) CreateIndex(ctx context.Context, tableName, indexName string, schema *types.Schema, expr *types.Evaluator) (interfaces.Index, error) {
	_, found := d.index[indexName]
	if found {
		panic(fmt.Errorf("index creation not synchronized: %s", indexName))
	}
	// TODO: unique
	// DONE: derive contents on creation
	// DONE: update contents on insertion
	// TODO: use contents on query... with _rowid?
	t, ok := d.table[tableName]
	if !ok {
		return nil, fmt.Errorf("table not found: %s", tableName)
	}
	tableSource := t.Source()
	tableSchema := tableSource.Schema()
	var indexToTableColumn []int
	for _, c := range schema.Columns {
		i := sql.FindColumnIndex(&tableSchema, c.Name)
		if i == nil {
			return nil, fmt.Errorf("column not found: %s", c.Name)
		}
		indexToTableColumn = append(indexToTableColumn, *i)
	}

	var exprColumnNameToIndex map[string]int
	if expr != nil {
		exprColumnNameToIndex = make(map[string]int)
		for input := range expr.Inputs {
			i := sql.FindColumnIndex(&tableSchema, input)
			if i == nil {
				return nil, fmt.Errorf("expression reference: column not found: %s", input)
			}
			exprColumnNameToIndex[input] = *i
		}
	}

	mi := &MemIndex{
		schema:                schema,
		table:                 t,
		expr:                  expr,
		exprColumnNameToIndex: exprColumnNameToIndex,
		indexToTableColumn:    indexToTableColumn,
		rows:                  make([]types.Row, 0),
		inputs:                make(map[string]colval.ColumnValue),
	}

	it := tableSource.RowIterator()
	needSort := false
	for {
		tableRow, err := it.Next()
		if err != nil {
			return nil, fmt.Errorf("table iterator: %w", err)
		}
		if tableRow == nil {
			fmt.Printf("XXX CREATE INDEX done rows\n")
			break
		}
		if mi.updateIndexForSourceRow(tableRow) {
			needSort = true
		}
	}
	if needSort {
		sort.Sort(&rowArraySort{mi.rows})
	}

	for i := range mi.rows {
		fmt.Printf("XXX CREATE INDEX after sorting, row %d: %+v\n", i, mi.rows[i])
	}
	d.index[indexName] = mi
	t.indices = append(t.indices, mi)
	return mi, nil
}

func (mi *MemIndex) updateIndexForSourceRow(tableRow *types.Row) bool {
	for exprCol, i := range mi.exprColumnNameToIndex {
		mi.inputs[exprCol] = (*tableRow)[i]
	}
	if mi.expr != nil {
		res := colval.ToBool(mi.expr.Func(mi.inputs))
		if res == nil || !*res {
			return false
		}
	}
	indexRow := make([]colval.ColumnValue, len(mi.indexToTableColumn))
	for i := range mi.indexToTableColumn {
		indexRow[i] = (*tableRow)[mi.indexToTableColumn[i]]
	}
	fmt.Printf("XXX CREATE INDEX with row %+v\n", indexRow)
	mi.rows = append(mi.rows, indexRow)
	return true
}

type rowArraySort struct {
	rows []types.Row
}

func (rs *rowArraySort) Len() int {
	return len(rs.rows)
}
func (rs *rowArraySort) Less(i, j int) bool {
	for c := range rs.rows[i] {
		if sql.ColumnValueLess(rs.rows[i][c], rs.rows[j][c]) {
			return true
		}
	}
	return false
}
func (rs *rowArraySort) Swap(i, j int) {
	rs.rows[i], rs.rows[j] = rs.rows[j], rs.rows[i]
}

func (d *Mem) DropIndex(ctx context.Context, name string) error {
	delete(d.index, name)
	return nil
}

func (d *Mem) GetIndex(ctx context.Context, name string) (interfaces.Index, bool, error) {
	i, found := d.index[name]
	return i, found, nil
}

func (i *MemIndex) Source() types.Source {
	fmt.Printf("XXX making MemIndex.Source for %d rows\n", i.rows)
	return types.Source{
		RowIterator: func() types.RowIterator {
			fmt.Printf("XXX hey, creating MemIndex iterator with %d rows\n", len(i.rows))
			return sql.NewRowArrayIterator(i.schema, i.rows)
		},
		// XXX TODO no reason not to just have a pointer to the schema...
		Schema: func() types.Schema {
			return *i.schema
		},
	}
}

func (d *Mem) CreateView(ctx context.Context, name, query string, schema *types.Schema, recreate bool) (interfaces.View, error) {
	_, found := d.view[name]
	if found && !recreate {
		panic(fmt.Errorf("view creation not synchronized: %s", name))
	}
	v := &MemView{
		schema: schema,
		query:  query,
	}
	d.view[name] = v
	return v, nil
}

func (d *Mem) DropView(ctx context.Context, name string) error {
	delete(d.view, name)
	return nil
}

func (d *Mem) GetView(ctx context.Context, name string) (interfaces.View, bool, error) {
	t, found := d.view[name]
	return t, found, nil
}

func (t *MemView) Source() types.Source {
	panic("unimpl")
}
