package db

import (
	"context"
	"encoding/gob"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/ksuid"

	"github.com/jrhy/mast/persist/s3test"
	"github.com/jrhy/s3db"

	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/driver/interfaces"
	"github.com/jrhy/sandbox/sql/types"
)

type S3DB struct {
	root   *s3db.DB
	closer func()
}

var _ interfaces.DB = &S3DB{}

type S3DBTable struct {
	ID     string
	schema *types.Schema
	db     *S3DB
}

func NewS3DB(ctx context.Context, url string) (*S3DB, error) {
	c, bucketName, closer := s3test.Client()
	cfg := s3db.Config{
		Storage: &s3db.S3BucketInfo{
			EndpointURL: c.Endpoint,
			BucketName:  bucketName,
			Prefix:      "prefix",
		},
		KeysLike:   "stringy",
		ValuesLike: types.Schema{},
	}
	gob.Register(colval.Int(5))
	gob.Register(colval.Text("hi"))
	gob.Register(colval.Blob(nil))
	gob.Register(colval.Null{})
	gob.Register(colval.Real(1.0))
	gob.Register(types.Row{})
	s, err := s3db.Open(ctx, c, cfg, s3db.OpenOptions{}, time.Now())
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	return &S3DB{
		root:   s,
		closer: closer,
	}, nil
}

func (d *S3DB) GetTable(ctx context.Context, name string) (interfaces.Table, bool, error) {
	var id string
	found, err := d.root.Get(ctx, "tableid-"+name, &id)
	if !found || err != nil {
		return nil, found, err
	}
	if id == "" {
		return nil, false, nil
	}
	var schema types.Schema
	found, err = d.root.Get(ctx, fmt.Sprintf("%s-schema", id), &schema)
	if err == nil && !found {
		err = fmt.Errorf("%s-schema missing", id)
	}
	if err != nil {
		return nil, false, err
	}
	return &S3DBTable{
		ID:     id,
		schema: &schema,
		db:     d,
	}, true, nil
}

func (d *S3DB) CreateTable(ctx context.Context, name string, schema *types.Schema) (interfaces.Table, error) {
	id := ksuid.New().String()
	t := time.Now()
	err := d.root.Set(ctx, t, fmt.Sprintf("tableid-%s", name), id)
	if err != nil {
		return nil, err
	}
	// TODO we don't want to store schema struct directly, the textual representation.
	err = d.root.Set(ctx, t, fmt.Sprintf("%s-schema", id), *schema)
	if err != nil {
		return nil, err
	}
	return &S3DBTable{
		ID:     id,
		schema: schema,
		db:     d,
	}, nil
}

func (d *S3DB) DropTable(ctx context.Context, name string) error {
	t, found, err := d.GetTable(ctx, name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("table '%s' not found", name)
	}
	err = d.root.Set(ctx, time.Now(), "tableid-"+name, "")
	if err != nil {
		return fmt.Errorf("s3db tableid set: %w", err)
	}
	err = d.root.Set(ctx, time.Now(), fmt.Sprintf("%s-dropped", (t.(*S3DBTable)).ID), true)
	if err != nil {
		return fmt.Errorf("s3db tableid dropped set: %w", err)
	}
	return nil
}

func (t *S3DBTable) Insert(ctx context.Context, rows []types.Row) error {
	when := time.Now()
	uid := ksuid.New()
	clone, err := t.db.root.Clone(ctx)
	if err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	for i := range rows {
		// TODO - adding a layer of indirection for per-column data will give more
		// flexibility for ALTER TABLE.
		// TODO - change behaviour depending on schema w.r.t. primary key / rowid.
		err := t.db.root.Set(ctx, when, fmt.Sprintf("%s-row-%s%d", t.ID, uid, i), rows[i])
		if err != nil {
			return fmt.Errorf("set: %w", err)
		}
	}
	_, err = t.db.root.Commit(ctx)
	if err != nil {
		t.db.root = clone
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (t *S3DBTable) Source() types.Source {
	return types.Source{
		RowIterator: func() types.RowIterator {
			prefix := fmt.Sprintf("%s-row-", t.ID)
			return &S3DBIterator{
				table:  t,
				last:   prefix,
				prefix: prefix,
			}
		},
		// TODO no reason not to just have a pointer to the schema...
		// TODO the iterator has a way to get the schema TOOOO... pick one.
		Schema: func() types.Schema {
			return *t.schema
		},
	}
}

type S3DBIterator struct {
	table  *S3DBTable
	last   string
	prefix string
}

func (i *S3DBIterator) Next() (*types.Row, error) {
	// XXX this dirty n^2 crap is due to the lack of a cursor interface in mast/s3db.
	// Using a channel and goroutine isn't great unless there's a good way to avoid
	// leaking the goroutine if the caller doesn't finish iterating.
	// TODO context
	var res *types.Row
	i.table.db.root.Diff(context.Background(), nil,
		func(key, myValue, fromValue interface{}) (keepGoing bool, err error) {
			if strings.Compare(key.(string), i.last) <= 0 {
				return true, nil
			}
			if !strings.HasPrefix(key.(string), i.prefix) {
				return false, nil
			}
			i.last = key.(string)
			row := myValue.(types.Row)
			res = &row
			return false, nil
		})
	return res, nil
}
func (i *S3DBIterator) Schema() *types.Schema { return i.table.schema }

func (d *S3DB) CreateView(ctx context.Context, name, query string, schema *types.Schema, recreate bool) (interfaces.View, error) {
	panic("unimpl")
}

func (d *S3DB) DropView(ctx context.Context, name string) error {
	panic("unimpl")
}

func (d *S3DB) GetView(ctx context.Context, name string) (interfaces.View, bool, error) {
	panic("unimpl")
}

func (d *S3DB) CreateIndex(ctx context.Context, tableName, indexName string, schema *types.Schema, expr *types.Evaluator) (interfaces.Index, error) {
	panic("unimpl")
}

func (d *S3DB) DropIndex(ctx context.Context, name string) error {
	panic("unimpl")
}

func (d *S3DB) GetIndex(ctx context.Context, name string) (interfaces.Index, bool, error) {
	panic("unimpl")
}
