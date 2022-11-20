package sasqlite

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/segmentio/ksuid"

	"github.com/jrhy/mast/persist/s3test"
	"github.com/jrhy/s3db"
	"github.com/jrhy/s3db/crdt"
	"github.com/jrhy/sandbox/parse"
	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	sqlTypes "github.com/jrhy/sandbox/sql/types"
)

var ErrSasqliteConstraintNotNull = errors.New("constraint: NOT NULL")
var ErrSasqliteConstraintPrimaryKey = errors.New("constraint: key not unique")
var ErrSasqliteConstraintUnique = errors.New("constraint: not unique")

type S3DB struct {
	Root   *s3db.DB
	closer func()
}

type Module struct{}

type VirtualTable struct {
	Ctx               context.Context
	cancelFunc        func()
	SchemaString      string
	schema            *sqlTypes.Schema
	ColumnIndexByName map[string]int
	ColumnNameByIndex map[int]string
	Tree              *S3DB
	txStart           *s3db.DB
	keyCol            int
	usesRowID         bool
	writeTime         *time.Time

	s3 s3Options
}

type Row struct {
	ColumnValues     map[string]ColumnValue
	Deleted          bool
	DeleteUpdateTime time.Time
}
type ColumnValue struct {
	UpdateTime time.Time
	Value      interface{}
}

func unquoteAll(s string) string {
	if len(s) == 0 {
		return ""
	}
	p := &parse.Parser{
		Remaining: s,
	}
	var cv colval.ColumnValue
	var res string
	for {
		if ok := sql.ColumnValueParser(&cv)(p); !ok {
			dbg("skippin unquote; using: %s\n", s)
			return s
		}
		res += cv.String()
		if len(p.Remaining) == 0 {
			break
		}
	}
	dbg("unquoted to: %s\n", res)
	return res
}

func New(tableName string, args []string) (*VirtualTable, error) {
	var err error

	var table = &VirtualTable{}

	dbg("CONNECT\n")
	table.Ctx = context.Background()
	if len(args) == 1 {
		return nil, errors.New(`
usage:
 deadline='<N>[s,m,h,d]',          timeout operations if they take too long
[s3_bucket='mybucket',]            defaults to in-memory bucket
[s3_endpoint='https://minio.example.com',]
                                   optional S3 endpoint (if not using AWS region)
[s3_prefix='/prefix',]             separate tables within a bucket
 schema='<colname> [type] [primary key] [not null]',
                                   table schema`)
		//write_time='19:33:11.413219'      attribute modification time, for idempotence, from request time
	}
	seen := map[string]struct{}{}
	for i := range args {
		s := strings.SplitN(args[i], "=", 2)
		if _, ok := seen[s[0]]; ok {
			return nil, fmt.Errorf("duplicated: %s", s[0])
		}
		seen[s[0]] = struct{}{}
		switch s[0] {
		case "deadline":
			d, err := time.ParseDuration(unquoteAll(s[1]))
			if err != nil {
				return nil, fmt.Errorf("deadline: %w", err)
			}
			t := time.Now().Add(d)
			table.Ctx, table.cancelFunc = context.WithDeadline(context.Background(), t)
		case "s3_bucket":
			table.s3.Bucket = unquoteAll(s[1])
		case "s3_endpoint":
			table.s3.Endpoint = unquoteAll(s[1])
		case "s3_prefix":
			table.s3.Prefix = unquoteAll(s[1])
		case "schema":
			err = convertSchema(unquoteAll(s[1]), table)
			if err != nil {
				return nil, fmt.Errorf("schema: %w", err)
			}
		default:
			dbg("skipping arg %s\n", args[i])
		}
	}

	table.Tree, err = newS3DB(table.Ctx, table.s3,
		fmt.Sprintf("%s-columns", tableName))
	if err != nil {
		return nil, fmt.Errorf("s3db: %w", err)
	}

	if table.SchemaString == "" {
		return nil, errors.New(`unspecified: schema='colname [type] [primary key] [not null], ...'`)
	}
	return table, nil
}

func convertSchema(s string, t *VirtualTable) error {
	schema, err := parseSchema(s)
	if err != nil {
		return err
	}
	if len(schema.PrimaryKey) > 1 {
		return fmt.Errorf("sqlite vtable primary key cannot be composite")
	}
	columnMap := map[string]struct{}{}
	for i := range schema.Columns {
		name := schema.Columns[i].Name
		if _, ok := columnMap[name]; ok {
			return fmt.Errorf("duplicate column: %s", name)
		}
		columnMap[schema.Columns[i].Name] = struct{}{}
	}
	t.usesRowID = true
	var keyColName string
	if len(schema.PrimaryKey) > 0 {
		keyColName = schema.PrimaryKey[0]
		if _, ok := columnMap[schema.PrimaryKey[0]]; !ok {
			return fmt.Errorf("no column definition for key: %s", keyColName)
		}
		t.usesRowID = false
	}
	s = "CREATE TABLE x("
	if t.usesRowID {
		s += "_rowid_ HIDDEN PRIMARY KEY NOT NULL, "
	}
	for i, c := range schema.Columns {
		if i > 0 {
			s += ", "
		}
		s += c.Name
		if c.DefaultType != "" {
			s += " " + c.DefaultType
		}
		if c.Name == keyColName {
			s += " PRIMARY KEY"
			t.keyCol = i
		}
		if c.Unique && i != t.keyCol {
			return fmt.Errorf("UNIQUE not supported for non-key column: %s", c.Name)
		}
		if c.Default != nil {
			return fmt.Errorf("DEFAULT not supported: %s", c.Name)
		}
		if c.NotNull {
			s += " NOT NULL"
		}
	}
	s += ") WITHOUT ROWID"

	t.SchemaString = s
	t.schema = schema

	t.ColumnIndexByName = map[string]int{}
	t.ColumnNameByIndex = map[int]string{}
	for i, col := range t.schema.Columns {
		t.ColumnIndexByName[col.Name] = i
		t.ColumnNameByIndex[i] = col.Name
	}
	return nil
}

func parseSchema(a string) (*sqlTypes.Schema, error) {
	var schema sqlTypes.Schema
	var errs []error
	p := &parse.Parser{
		Remaining: a,
	}
	res := sql.Schema(&schema, &errs)(p)
	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to parse: %+v", errs)
	}
	if !res {
		return nil, fmt.Errorf("failed to parse")
	}
	if !parse.End()(p) {
		return nil, fmt.Errorf("failed to parse at: '%s'", p.Remaining)
	}
	// b, _ := json.Marshal(schema)
	// dbg("woo, neat schema! %s\n", string(b))
	return &schema, nil
}

type Op int

const (
	OpIgnore = iota
	OpEQ
	OpLT
	OpLE
	OpGE
	OpGT
)

type IndexInput struct {
	Op          Op
	ColumnIndex int
}
type OrderInput struct {
	Column int
	Desc   bool
}
type IndexOutput struct {
	EstimatedCost  float64
	Used           []bool
	AlreadyOrdered bool
	IdxNum         int
	IdxStr         string
}

func (c *VirtualTable) BestIndex(input []IndexInput, order []OrderInput) (*IndexOutput, error) {
	cost := float64(c.Tree.Root.Size())
	dbg("BESTINDEX %+v\n = %f\n", input, cost)
	out := &IndexOutput{
		EstimatedCost: cost,
		Used:          make([]bool, len(input)),
	}
	for i := range input {
		if input[i].Op == OpIgnore {
			continue
		}
		if input[i].ColumnIndex != c.keyCol {
			continue
		}
		out.Used[i] = true
		cs := strconv.FormatInt(int64(input[i].Op), 10)
		if out.IdxStr == "" {
			out.IdxStr = cs
		} else {
			out.IdxStr += "," + cs
		}
		out.EstimatedCost /= 2.0
	}
	out.AlreadyOrdered = true
	var desc *bool
	for i := range order {
		if order[i].Column != c.keyCol {
			out.AlreadyOrdered = false
		}
		if desc != nil {
			return nil, errors.New("order specified multiple times")
		}
		v := order[i].Desc
		desc = &v
	}
	if desc == nil {
		a := false
		desc = &a
	}
	if *desc {
		out.IdxStr = "desc " + out.IdxStr
	} else {
		out.IdxStr = "asc  " + out.IdxStr
	}
	return out, nil
}

func (c *VirtualTable) Open() (*Cursor, error) {
	dbg("OPEN\n")
	cursor, err := c.Tree.Root.Cursor(c.Ctx)
	if err != nil {
		return nil, err
	}
	return &Cursor{
		t:      c,
		cursor: cursor,
	}, nil
}

func (c *VirtualTable) Disconnect() error {
	dbg("DISCONNECT\n")

	c.Tree.Root.Cancel()
	if c.Tree.closer != nil {
		c.Tree.closer()
	}
	c.Tree = nil
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	c.Ctx = nil

	return nil
}

type Cursor struct {
	t          *VirtualTable
	currentKey *Key
	currentRow *Row
	cursor     *s3db.Cursor
	desc       bool
	eof        bool
	ops        []Op
	operands   []*Key
	//REMOVE rowid   int64
	min, max     *Key
	gtMin, ltMax bool
}

func (c *Cursor) Next() error {
	if c.eof {
		dbg("NEXT EOF\n")
		return nil
	}
	for {
		k, v, ok := c.cursor.Get()
		dbg("next got: %+v %+v %+v\n", k, v, ok)
		if !ok {
			c.eof = true
			return nil
		}
		// stop at end of range
		if !c.desc { // order asc
			if c.max != nil {
				cmp := k.(*Key).Order(c.max)
				if c.ltMax && cmp >= 0 || cmp > 0 {
					c.eof = true
					return nil
				}
			}
		} else { // order desc
			if c.min != nil {
				cmp := k.(*Key).Order(c.min)
				if c.gtMin && cmp <= 0 || cmp < 0 {
					c.eof = true
					return nil
				}
			}
		}
		skip := false
		if !c.desc {
			if c.min != nil && c.gtMin && k.(*Key).Order(c.min) == 0 {
				skip = true
				c.gtMin = false
			}
		} else {
			if c.max != nil && c.ltMax && k.(*Key).Order(c.max) == 0 {
				skip = true
				c.ltMax = false
			}
		}
		if v.Value.(Row).Deleted {
			skip = true
		}
		if !c.desc {
			err := c.cursor.Forward(c.t.Ctx)
			if err != nil {
				return fmt.Errorf("forward: %w", err)
			}
		} else {
			err := c.cursor.Backward(c.t.Ctx)
			if err != nil {
				return fmt.Errorf("backward: %w", err)
			}
		}
		if skip {
			dbg("skip %+v\n", k)
			continue
		}
		row := v.Value.(Row)
		c.currentRow = &row
		c.currentKey = k.(*Key)
		return nil
	}
}

func (c *Cursor) Column(i int) (interface{}, error) {
	dbg("column %d\n", i)
	if c.currentRow.Deleted {
		return nil, fmt.Errorf("accessing deleted row")
	}
	if i == c.t.keyCol {
		return c.currentKey.Value(), nil
	}
	if cv, ok := c.currentRow.ColumnValues[c.t.ColumnNameByIndex[i]]; ok {
		return cv.Value, nil
	}
	return nil, nil
}

func (c *Cursor) Filter(idxStr string, val []interface{}) error {
	dbg("FILTER idxStr=%+v val=%+v\n", idxStr, val)
	c.ops = make([]Op, len(val))
	c.operands = make([]*Key, len(val))
	switch idxStr[:4] {
	case "desc":
		c.desc = true
	case "asc ":
		c.desc = false
	default:
		return errors.New("malformed filter index string: " + idxStr)
	}
	idxStr = idxStr[5:]
	for i, s := range strings.Split(idxStr, ",") {
		if s == "" {
			continue
		}
		opInt, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			res := fmt.Errorf("parse op %s: %w", s, err)
			return res
		}
		// TODO: how about a test using a > 3 OR a > 5, see which filters get used
		op := Op(opInt)
		c.ops[i] = op
		c.operands[i] = NewKey(val[i])
		if op == OpLT || op == OpLE || op == OpEQ {
			if c.max == nil || c.max != nil && c.operands[i].Order(c.max) < 0 {
				c.max = c.operands[i]
				c.ltMax = op == OpLT
			}
		}
		if op == OpGT || op == OpGE || op == OpEQ {
			if c.min == nil || c.min != nil && c.operands[i].Order(c.min) > 0 {
				c.min = c.operands[i]
				c.gtMin = op == OpGT
			}
		}
	}
	var err error
	if !c.desc {
		if c.min != nil {
			err = c.cursor.Ceil(c.t.Ctx, c.min)
		} else {
			err = c.cursor.Min(c.t.Ctx)
		}
	} else {
		if c.max != nil {
			err = c.cursor.Ceil(c.t.Ctx, c.max)
		} else {
			err = c.cursor.Max(c.t.Ctx)
		}
	}
	if err != nil {
		return fmt.Errorf("cursor: %w", err)
	}
	c.currentKey = nil
	c.currentRow = nil
	c.eof = false
	res := c.Next()
	dbg("FILTER RESET: err=%v\n", res)
	return res
}

func (c *Cursor) Rowid() (int64, error) {
	return 0, errors.New("rowid: invalid for WITHOUT ROWID table")
}
func (c *Cursor) Eof() bool { return c.eof }
func (c *Cursor) Close() error {
	dbg("CLOSE\n")
	return nil
}

func (c *VirtualTable) Insert(values map[int]interface{}) (int64, error) {
	dbg("INSERT ")
	if _, ok := values[c.keyCol]; !ok {
		return 0, errors.New("insert without key")
	}
	t := c.updateTime()
	var key interface{}
	if c.usesRowID {
		r, err := ksuid.NewRandomWithTime(t)
		if err != nil {
			return 0, fmt.Errorf("ksuid: %w", err)
		}
		key = r.String()
	} else {
		key = values[c.keyCol]
		if key == nil {
			return 0, ErrSasqliteConstraintNotNull
		}
	}
	dbg("%T %+v\n", key, key)
	var old, new Row
	ok, err := c.Tree.Root.Get(c.Ctx, NewKey(key), &old)
	if err != nil {
		return 0, fmt.Errorf("get: %w", err)
	}
	if ok && (!old.Deleted || !old.DeleteUpdateTime.Before(t)) {
		return 0, ErrSasqliteConstraintPrimaryKey
	}
	new.Deleted = false
	new.DeleteUpdateTime = t
	if new.ColumnValues == nil {
		new.ColumnValues = make(map[string]ColumnValue)
	}
	for i, v := range values {
		if i == c.keyCol {
			continue
		}
		cv := ColumnValue{
			Value:      v,
			UpdateTime: t,
		}
		new.ColumnValues[c.ColumnNameByIndex[i]] = cv
		dbg("SET %d %v=%v\n", i, key, v)
	}
	merged := mergeRows(key, old, new)
	err = c.Tree.Root.Set(c.Ctx, t, NewKey(key), merged)
	if err != nil {
		return 0, fmt.Errorf("set: %w", err)
	}
	return 0, nil
}

func (c *VirtualTable) Update(key interface{}, values map[int]interface{}) error {
	dbg("UPDATE ")
	if key == nil {
		key = values[c.keyCol]
	}
	if key == nil {
		return errors.New("no key set")
	}
	t := c.updateTime()
	var old, new Row
	ok, err := c.Tree.Root.Get(c.Ctx, NewKey(key), &old)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	if !ok || old.Deleted {
		return nil
	}
	new.ColumnValues = make(map[string]ColumnValue)
	colName := c.ColumnNameByIndex[c.keyCol]
	cv := old.ColumnValues[colName]
	if cv.UpdateTime.Before(t) {
		cv.Value = key
		cv.UpdateTime = t
	}
	new.ColumnValues[colName] = cv
	for i, v := range values {
		if i == c.keyCol {
			continue
		}
		dbg("SET %d %v=%v\n", i, key, v)
		colName := c.ColumnNameByIndex[i]
		cv := old.ColumnValues[colName]
		if cv.UpdateTime.Before(t) {
			cv.Value = v
			cv.UpdateTime = t
		}
		new.ColumnValues[colName] = cv
	}
	merged := mergeRows(key, old, new)
	err = c.Tree.Root.Set(c.Ctx, t, NewKey(key), merged)
	if err != nil {
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

func (c *VirtualTable) updateTime() time.Time {
	if c.writeTime != nil {
		return *c.writeTime
	}
	return time.Now()
}

func (c *VirtualTable) Delete(key interface{}) error {
	dbg("DELETE ")
	dbg("nochange=%v %s %+v\n", key, key, key)
	var old, new Row
	_, err := c.Tree.Root.Get(c.Ctx, NewKey(key), &old)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	}
	t := c.updateTime()
	new.DeleteUpdateTime = t
	new.Deleted = true
	merged := mergeRows(key, old, new)
	err = c.Tree.Root.Set(c.Ctx, t, NewKey(key), merged)
	if err != nil {
		return fmt.Errorf("set: %w", err)
	}
	return nil
}

type s3Options struct {
	Bucket   string
	Endpoint string
	Prefix   string
}

func getS3(endpoint string) (*s3.S3, error) {
	config := aws.Config{}
	if endpoint != "" {
		config.Endpoint = &endpoint
		config.S3ForcePathStyle = aws.Bool(true)
	}

	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}

	return s3.New(sess), nil
}

func newS3DB(ctx context.Context, s3opts s3Options, subdir string) (*S3DB, error) {
	var err error
	var c s3db.S3Interface
	var closer func()
	if (s3opts == s3Options{}) {
		var bucketName string
		var s3Client *s3.S3
		s3Client, bucketName, closer = s3test.Client()
		s3opts = s3Options{
			Endpoint: s3Client.Endpoint,
			Bucket:   bucketName,
		}
		c = s3Client
	} else {
		c, err = getS3(s3opts.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("s3 client: %w", err)
		}
	}
	path := strings.TrimPrefix(strings.TrimSuffix(s3opts.Prefix, "/"), "/") + "/" + strings.TrimPrefix(subdir, "/")
	// TODO enable to observe s3db bug around delete/merge needing squishing
	if false && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	cfg := s3db.Config{
		Storage: &s3db.S3BucketInfo{
			EndpointURL: s3opts.Endpoint,
			BucketName:  s3opts.Bucket,
			Prefix:      path,
		},
		KeysLike:    &Key{},
		ValuesLike:  Row{},
		CustomMerge: mergeValues,
	}
	s, err := s3db.Open(ctx, c, cfg, s3db.OpenOptions{}, time.Now())
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	dbg("%s size:%d\n", subdir, s.Size())
	return &S3DB{
		Root:   s,
		closer: closer,
	}, nil
}

// REMOVE var maxTime = time.Unix(1<<63-62135596801, 999999999)

func mergeValues(_ interface{}, i1, i2 crdt.Value) crdt.Value {
	if i1.Tombstoned() || i2.Tombstoned() {
		panic("not expecting tombstones")
	}
	resp := crdt.LastWriteWins(&i1, &i2)
	res := *resp
	res.Value = mergeRows(nil, i1.Value.(Row), i2.Value.(Row))
	return res
}

func mergeRows(_ interface{}, r1, r2 Row) Row {
	res := Row{
		ColumnValues: map[string]ColumnValue{},
	}
	var resetValuesBefore time.Time
	if r1.DeleteUpdateTime.Before(r2.DeleteUpdateTime) {
		res.Deleted = r2.Deleted
		res.DeleteUpdateTime = r2.DeleteUpdateTime
		if r1.Deleted {
			if !r2.Deleted {
				resetValuesBefore = r2.DeleteUpdateTime
			}
		}
	} else {
		res.Deleted = r1.Deleted
		res.DeleteUpdateTime = r1.DeleteUpdateTime
		if !r1.Deleted {
			if r2.Deleted {
				resetValuesBefore = r1.DeleteUpdateTime
			}
		}
	}
	allKeys := make(map[string]struct{})
	for k := range r1.ColumnValues {
		allKeys[k] = struct{}{}
	}
	for k := range r2.ColumnValues {
		allKeys[k] = struct{}{}
	}

	for k := range allKeys {
		if res.Deleted {
			res.ColumnValues[k] = ColumnValue{UpdateTime: res.DeleteUpdateTime, Value: nil}
			continue
		}
		v1, inR1 := r1.ColumnValues[k]
		v2, inR2 := r2.ColumnValues[k]
		if !inR1 {
			res.ColumnValues[k] = hideDeletedValue(v2, resetValuesBefore)
			continue
		}
		if !inR2 {
			res.ColumnValues[k] = hideDeletedValue(v1, resetValuesBefore)
			continue
		}
		if v1.UpdateTime.Before(v2.UpdateTime) {
			res.ColumnValues[k] = hideDeletedValue(v2, resetValuesBefore)
		} else {
			res.ColumnValues[k] = hideDeletedValue(v1, resetValuesBefore)
		}
	}

	return res
}

func hideDeletedValue(cv ColumnValue, resetValuesBefore time.Time) ColumnValue {
	if cv.UpdateTime.Before(resetValuesBefore) {
		return ColumnValue{UpdateTime: resetValuesBefore, Value: nil}
	}
	return cv
}

func (c *VirtualTable) Begin() error {
	dbg("BEGIN\n")
	var err error
	if c.txStart != nil {
		panic("transaction already in progress")
	}
	c.txStart, err = c.Tree.Root.Clone(c.Ctx)
	if err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	return nil
}

func (c *VirtualTable) Commit() error {
	dbg("COMMIT\n")
	_, err := c.Tree.Root.Commit(c.Ctx)
	if err != nil {
		return fmt.Errorf("commit tree: %w", err)
	}
	c.txStart = nil
	return nil
}

func (c *VirtualTable) Rollback() error {
	dbg("ROLLBACK\n")
	c.Tree.Root.Cancel()
	c.Tree.Root = c.txStart
	c.txStart = nil
	return nil
}

func dbg(f string, v ...interface{}) {
	if false {
		fmt.Printf(f, v...)
	}
}
