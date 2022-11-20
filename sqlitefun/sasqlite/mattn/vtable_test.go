//go:build sqlite_vtable

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jrhy/mast/persist/s3test"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func init() {
	sql.Register("sqlite3_with_extensions", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.CreateModule("sasqlite", &Module{})
		},
	})
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

func getDB() *sql.DB {
	db, err := sql.Open("sqlite3_with_extensions", ":memory:")
	if err != nil {
		panic(err)
	}
	return db
}

func getDBBucket() (*sql.DB, string, string) {
	var s3Bucket string
	var s3Endpoint string

	os.Setenv("AWS_REGION", "dummy")
	const UsingLocalMinIO = false
	if UsingLocalMinIO {
		os.Setenv("AWS_ACCESS_KEY_ID", "minioadmin")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "miniopassword")
		s3Bucket = "bucket"
		s3Endpoint = "http://127.0.0.1:9091"
	} else {
		os.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
		c, bucketName, _ := s3test.Client()
		s3Bucket = bucketName
		s3Endpoint = c.Endpoint
	}

	return getDB(), s3Bucket, s3Endpoint
}

func mustGetRows(r *sql.Rows) [][]interface{} {
	cols, err := r.Columns()
	if err != nil {
		panic(err)
	}
	var rows [][]interface{}
	for r.Next() {
		row := make([]interface{}, len(cols))
		for i := range row {
			var q interface{}
			row[i] = &q
		}
		err = r.Scan(row...)
		if err != nil {
			panic(fmt.Errorf("scan: %w", err))
		}
		rows = append(rows, row)
	}
	return rows
}

func Test1(t *testing.T) {
	db, s3Bucket, s3Endpoint := getDBBucket()
	defer db.Close()
	_, err := db.Exec(fmt.Sprintf(`create virtual table t1 using sasqlite (
s3_bucket='%s',
s3_endpoint='%s',
s3_prefix='t1',
schema='a primary key, b')`,
		s3Bucket, s3Endpoint))
	require.NoError(t, err)
	_, err = db.Exec("delete from t1;")
	require.NoError(t, err)
	_, err = db.Exec("insert into t1 values ('v1','v1b'),('v2','v2b'),('v3','v3b');")
	require.NoError(t, err)
	_, err = db.Exec("update t1 set a='v1c' where b='v1b';")
	require.NoError(t, err)
	_, err = db.Exec("delete from t1 where b='v2b';")
	require.NoError(t, err)
	require.Equal(t,
		`[["v1","v1b"],`+
			`["v3","v3b"]]`,
		mustQueryToJSON(db, "select * from t1;"))
}

func expand(row []interface{}) []interface{} {
	res := []interface{}{}
	for i := range row {
		for _, v := range row[i].([]interface{}) {
			res = append(res, v)
		}
	}
	return res
}
func populateTwoTables(db *sql.DB, s3Bucket, s3Endpoint, tablePrefix, schema string, row ...interface{}) (string, string, error) {

	regTableName := tablePrefix + "_reg"
	virtualTableName := tablePrefix + "_virtual"
	_, err := db.Exec(fmt.Sprintf(`create table %s(%s)`,
		regTableName, schema))
	if err != nil {
		return "", "", fmt.Errorf("create %s: %w", regTableName, err)
	}

	_, err = db.Exec(fmt.Sprintf(`create virtual table %s using sasqlite (
s3_bucket='%s',
s3_endpoint='%s',
s3_prefix='%s',
schema='%s')`, virtualTableName,
		s3Bucket, s3Endpoint, virtualTableName, schema))
	if err != nil {
		return "", "", fmt.Errorf("create %s: %w", virtualTableName, err)
	}

	valuesStr := "values"
	for i := range row {
		if i > 0 {
			valuesStr += ","
		}
		valuesStr += "("
		for j := range row[i].([]interface{}) {
			if j > 0 {
				valuesStr += ","
			}
			valuesStr += "?"
		}
		valuesStr += ")"
	}
	_, err = db.Exec(fmt.Sprintf("insert into %s %s", regTableName, valuesStr), expand(row)...)
	if err != nil {
		return "", "", fmt.Errorf("insert %s: %w", regTableName, err)
	}
	_, err = db.Exec(fmt.Sprintf("insert into %s %s", virtualTableName, valuesStr), expand(row)...)
	if err != nil {
		return "", "", fmt.Errorf("insert %s: %w", virtualTableName, err)
	}

	return regTableName, virtualTableName, nil
}

func row(cols ...interface{}) interface{} {
	return interface{}(cols)
}

func requireSelectEquiv(t *testing.T, db *sql.DB, regTable, virtualTable, where, expectedJSON string) {
	require.Equal(t,
		expectedJSON,
		mustQueryToJSON(db, fmt.Sprintf("select * from %s %s", regTable, where)))
	require.Equal(t,
		expectedJSON,
		mustQueryToJSON(db, fmt.Sprintf("select * from %s %s", virtualTable, where)))
}
func TestBestIndex(t *testing.T) {
	db, s3Bucket, s3Endpoint := getDBBucket()
	defer db.Close()
	regTable, sasqTable, err := populateTwoTables(db, s3Bucket, s3Endpoint,
		"index", "a primary key",
		row(1),
		row(3),
		row(2),
	)
	require.NoError(t, err)
	t.Run("s3dbKeySorted", func(t *testing.T) {
		require.Equal(t,
			"[[1],[2],[3]]",
			mustQueryToJSON(db, fmt.Sprintf(`select a from %s`, sasqTable)))
	})
	sqliteEquiv := func(where, expectedJSON string) func(*testing.T) {
		return func(t *testing.T) {
			requireSelectEquiv(t, db, regTable, sasqTable,
				where, expectedJSON)
		}
	}
	t.Run("gt_asc", sqliteEquiv("where a>2", "[[3]]"))
	t.Run("ge_asc", sqliteEquiv("where a>=2", "[[2],[3]]"))
	t.Run("eq_asc", sqliteEquiv("where a=2", "[[2]]"))
	t.Run("le_asc", sqliteEquiv("where a<=2", "[[1],[2]]"))
	t.Run("lt_asc", sqliteEquiv("where a<2", "[[1]]"))

	t.Run("gt_desc", sqliteEquiv("where a>2 order by 1 desc", "[[3]]"))
	t.Run("ge_desc", sqliteEquiv("where a>=2 order by 1 desc", "[[3],[2]]"))
	t.Run("eq_desc", sqliteEquiv("where a=2 order by 1 desc", "[[2]]"))
	t.Run("le_desc", sqliteEquiv("where a<=2 order by 1 desc", "[[2],[1]]"))
	t.Run("lt_desc", sqliteEquiv("where a<2 order by 1 desc", "[[1]]"))

	t.Run("and_asc", sqliteEquiv("where a>1 and a<3", "[[2]]"))
}

func dump(rows *sql.Rows) error {
	colNames, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("columns: %w", err)
	}
	cols := make([]interface{}, len(colNames))
	for i := range colNames {
		var ef interface{}
		cols[i] = &ef
	}
	for rows.Next() {
		err = rows.Scan(cols...)
		if err != nil {
			return err
		}
		s := "ROW: "
		for i := range cols {
			s += fmt.Sprintf("%+v ", *cols[i].(*interface{}))
		}
		fmt.Println(s)
	}
	return nil
}

func mustQueryToJSON(db *sql.DB, query string) string {
	rows, err := db.Query(query)
	if err != nil {
		panic(fmt.Errorf("%s: %w", query, err))
	}
	defer rows.Close()
	return mustJSON(mustGetRows(rows))
}

func TestSortOrder(t *testing.T) {
	db, s3Bucket, s3Endpoint := getDBBucket()
	defer db.Close()
	_, err := db.Exec(fmt.Sprintf(`create virtual table sortfun using sasqlite (
s3_bucket='%s',
s3_endpoint='%s',
s3_prefix='sortfun',
schema='a primary key')`,
		s3Bucket, s3Endpoint))
	require.NoError(t, err)
	_, err = db.Exec(`insert into sortfun values (?), (?), (?), (?)`,
		[]byte("blob"),
		"text",
		3.14,
		3,
	)
	require.NoError(t, err)
	require.Equal(t,
		`[["integer"],["real"],["text"],["blob"]]`,
		mustQueryToJSON(db, `select typeof(a) from sortfun`))

	_, err = db.Exec(`create table sortfun_native(a primary key) without rowid`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into sortfun_native values (?), (?), (?), (?)`,
		[]byte("blob"),
		"text",
		3.14,
		3,
	)
	require.NoError(t, err)
	require.Equal(t,
		`[["integer"],["real"],["text"],["blob"]]`,
		mustQueryToJSON(db, `select typeof(a) from sortfun_native`))
}

func TestNullPrimaryKey(t *testing.T) {
	db, s3Bucket, s3Endpoint := getDBBucket()
	defer db.Close()
	_, err := db.Exec(fmt.Sprintf(`create virtual table nullkey using sasqlite (
s3_bucket='%s',
s3_endpoint='%s',
s3_prefix='nullkey',
schema='a primary key')`,
		s3Bucket, s3Endpoint))
	require.NoError(t, err)
	_, err = db.Exec(`insert into nullkey values (null);`)
	require.Error(t, err, "constraint failed")
}

func TestNullValue(t *testing.T) {
	db, s3Bucket, s3Endpoint := getDBBucket()
	_, err := db.Exec(fmt.Sprintf(`create virtual table nullval using sasqlite (
s3_bucket='%s',
s3_endpoint='%s',
s3_prefix='nullval',
schema='a')`,
		s3Bucket, s3Endpoint))
	require.NoError(t, err)
	_, err = db.Exec(`insert into nullval values (null);`)
	require.NoError(t, err)
	require.Equal(t,
		`[["null",null]]`,
		mustQueryToJSON(db, `select typeof(a), a from nullval`))
}
