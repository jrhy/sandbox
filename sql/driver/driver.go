package driver

import (
	"context"
	stdsql "database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/driver/db"
	"github.com/jrhy/sandbox/sql/driver/interfaces"
	"github.com/jrhy/sandbox/sql/types"
	//"github.com/segmentio/ksuid"
)

func init() {
	stdsql.Register("sandbox", &d{})
}

type d struct {
	sources map[string]types.Source
	indexes map[string]map[string]types.Source
	l       sync.Mutex
	db      interfaces.DB
}
type connector struct {
	driver *d
}
type conn struct {
	d *d
}
type stmt struct {
	Expression *types.Expression
	d          *d
}
type rows struct {
	expression *types.Expression
	res        chan *types.Row
	errors     chan error
	ctx        context.Context
	cancel     func()
}

var _ driver.Connector = &connector{}

func (c *connector) Connect(_ context.Context) (driver.Conn, error) {
	return nil, errors.New("TODO")
}
func (c *connector) Driver() driver.Driver {
	return c.driver
}

func (d *d) Open(name string) (driver.Conn, error) {
	return &conn{d: d}, nil
}

func (conn *conn) Begin() (driver.Tx, error) {
	return nil, nil
}
func (conn *conn) Close() error {
	return nil
}
func (conn *conn) Prepare(query string) (driver.Stmt, error) {
	fmt.Printf(`Preparing "%s", with sources %+v%s`, query, conn.d.sources, "\n")
	exp, err := sql.Parse(conn.d.sources, query)
	if err != nil {
		return nil, err
	}
	fmt.Printf(`Prepared "%s"\n`, query)
	fmt.Printf("Expression: %+v\n", exp)
	return stmt{
		Expression: exp,
		d:          conn.d,
	}, nil
}

func (s stmt) Close() error {
	return nil
}
func (s stmt) NumInput() int {
	return -1
}
func (s stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), args)
}
func (s stmt) ExecContext(ctx context.Context, args []driver.Value) (driver.Result, error) {
	switch {
	case s.Expression.Create != nil:
		return s.create(ctx)
	case s.Expression.Drop != nil:
		return s.drop(ctx)
	case s.Expression.Insert != nil:
		return s.insert(ctx)
	case s.Expression.Select != nil:
		return nil, errors.New("exec: not applicable; use .Query() instead")
	default:
	}
	return nil, fmt.Errorf("unimplemented")
}

func (s stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), args)
}

func (s stmt) QueryContext(ctx context.Context, args []driver.Value) (driver.Rows, error) {
	rowsContext, cancel := context.WithCancel(ctx)
	rows := rows{
		expression: s.Expression,
		res:        make(chan *types.Row),
		ctx:        rowsContext,
		cancel:     cancel,
	}

	// var err error
	// var orderBy []types.OrderBy
	// var temporaryTable interfaces.Table
	if s.Expression.Select != nil && len(s.Expression.Select.OrderBy) > 0 {
		return nil, fmt.Errorf("ORDER BY not implemented yet")
		/*
			// TODO handle this with a temporary view
			orderBy = s.Expression.Select.OrderBy
			s.Expression.Select.OrderBy = nil
			schema := &types.Schema{
				Name:    "temp_" + ksuid.New().String(),
				Columns: append([]types.SchemaColumn{}, s.Expression.Select.Schema.Columns...),
			}
			temporaryTable, err = s.d.db.CreateTable(ctx, schema.Name, schema)
			if err != nil {
				return nil, fmt.Errorf("createTemporaryTable: %w", err)
			}
			fmt.Printf("orderBy: %+v\n", orderBy)
			fmt.Printf("temporaryTable: %+v\n", temporaryTable)
		*/
	}

	initialError := make(chan error)
	go func() {
		first := true
		err := sql.Eval(s.Expression, s.d.sources, func(row *types.Row) error {
			if first {
				initialError <- nil
				first = false
			}
			fmt.Printf("CB SENDING row %+v\n", row)
			select {
			case <-ctx.Done():
				fmt.Printf("CB IGNORING ROW, CTX DONE\n")
				return ctx.Err()
			case rows.res <- row:
				fmt.Printf("CB SENT row %+v\n", row)
			}
			return nil
		})
		if err != nil {
			if first {
				initialError <- err
			} else {
				rows.errors <- err
			}
		} else if first {
			initialError <- nil
		}
		fmt.Printf("CB DONE rows\n")
		rows.cancel()
	}()
	if err := <-initialError; err != nil {
		return nil, fmt.Errorf("eval: %w", err)
	}
	return rows, nil
}

func (s stmt) create(ctx context.Context) (driver.Result, error) {
	var rows []types.Row
	s.d.l.Lock()
	defer s.d.l.Unlock()
	if s.d.db == nil {
		var err error
		s.d.db = db.NewMem()
		//s.d.db, err = db.NewS3DB(ctx, "placeholder")
		if err != nil {
			return nil, err
		}
		if s.d.db == nil {
			panic("goo")
		}
	}
	if s.Expression.Create.Index != nil {
		indexName := s.Expression.Create.Schema.Name
		if len(indexName) == 0 {
			return nil, fmt.Errorf("creating index with empty schema name")
		}
		if _, found, err := s.d.db.GetIndex(ctx, indexName); found || err != nil {
			if found {
				return nil, fmt.Errorf("create index: '%s' already exists", indexName)
			}
			return nil, fmt.Errorf("create index: get index '%s': %w", indexName, err)
		}
		tableName := s.Expression.Create.Index.Table
		if len(tableName) == 0 {
			return nil, fmt.Errorf("creating index with empty table name")
		}
		if _, found, err := s.d.db.GetTable(ctx, tableName); !found || err != nil {
			if !found {
				return nil, fmt.Errorf("create index: table '%s' does not exist", tableName)
			}
			return nil, fmt.Errorf("create index: get table '%s': %w", tableName, err)
		}
		if s.d.indexes == nil {
			s.d.indexes = make(map[string]map[string]types.Source)
		}
		if s.d.indexes[tableName] == nil {
			s.d.indexes[tableName] = make(map[string]types.Source)
		}
		i, err := s.d.db.CreateIndex(ctx, tableName, indexName, &s.Expression.Create.Schema, s.Expression.Create.Index.Expr)
		if err != nil {
			return nil, fmt.Errorf("create index: %w", err)
		}
		source := i.Source()
		s.d.indexes[tableName][indexName] = source
		// XXX rethink whether indexes should be exposed as sources, once they are working haha
		s.d.sources[indexName] = source
		source.RowIterator()
		fmt.Printf("XXX hey, added source for %s\n", indexName)
		return nil, nil
	}
	if s.Expression.Create.Query != nil {
		err := sql.EvalSelect(s.Expression.Create.Query, s.d.sources, func(row *types.Row) error {
			// TODO
			rows = append(rows, *row)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("eval: %w", err)
		}
	}
	if s.d.sources == nil {
		s.d.sources = make(map[string]types.Source)
	}
	tableName := s.Expression.Create.Schema.Name
	if len(tableName) == 0 {
		return nil, errors.New("creating table with empty schema name")
	}
	fmt.Printf("XXX creating table with schema name: '%s'\n", tableName)
	if _, found, err := s.d.db.GetTable(ctx, tableName); found || err != nil {
		if found {
			return nil, fmt.Errorf("create table: '%s' already exists", tableName)
		}
		return nil, fmt.Errorf("create table: get table '%s': %w", tableName, err)
	}
	t, err := s.d.db.CreateTable(ctx, tableName, &s.Expression.Create.Schema)
	if err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}
	if err := t.Insert(ctx, rows); err != nil {
		return nil, fmt.Errorf("insert: %w", err)
	}
	s.d.sources[tableName] = t.Source()
	return nil, nil
}

func (s stmt) drop(ctx context.Context) (driver.Result, error) {
	s.d.l.Lock()
	defer s.d.l.Unlock()
	tableName := s.Expression.Drop.TableRef.Table
	if len(tableName) == 0 {
		panic("dropping table with empty schema name")
	}
	if s.d.db == nil || s.d.sources == nil {
		return nil, fmt.Errorf("drop: '%s' not found", tableName)
	}
	if _, found, err := s.d.db.GetTable(ctx, tableName); !found || err != nil {
		if !found {
			return nil, fmt.Errorf("drop: '%s' not found", tableName)
		}
		return nil, fmt.Errorf("drop: get table '%s': %w", tableName, err)
	}
	err := s.d.db.DropTable(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("drop table: %w", err)
	}
	delete(s.d.sources, tableName)
	return nil, nil
}

func (s stmt) insert(ctx context.Context) (driver.Result, error) {
	tableName := s.Expression.Insert.Schema.Name
	s.d.l.Lock()
	defer s.d.l.Unlock()
	t, found, err := s.d.db.GetTable(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("get table %s: %w", tableName, err)
	}
	if !found {
		return nil, fmt.Errorf("table not found: %s", tableName)
	}
	var newRows []types.Row
	if s.Expression.Insert.Query != nil {
		err := sql.EvalSelect(s.Expression.Insert.Query, s.d.sources, func(row *types.Row) error {
			newRows = append(newRows, *row)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("eval: %w", err)
		}
	} else if s.Expression.Insert.Values != nil {
		newRows = s.Expression.Insert.Values.Rows
	}
	err = t.Insert(ctx, newRows)
	return nil, err
}

func (r rows) Close() error {
	r.cancel()
	return nil
}
func (r rows) Columns() []string {
	if r.expression.Select == nil {
		panic("non-select statement unimplemented")
	}
	schemaColumns := r.expression.Select.Schema.Columns
	res := make([]string, len(schemaColumns))
	for i := range schemaColumns {
		res[i] = schemaColumns[i].Name
	}
	return res
}
func (r rows) Next(dest []driver.Value) error {
	fmt.Printf("rows.Next() in select\n")
	select {
	case err := <-r.errors:
		return err
	case <-r.ctx.Done():
		return r.ctx.Err()
	case row, ok := <-r.res:
		if !ok {
			return io.EOF
		}
		for i := range *row {
			dest[i] = colval.ToGo((*row)[i])
		}
		return nil
	}
}
