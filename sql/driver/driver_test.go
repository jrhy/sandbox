package driver_test

import (
	stdsql "database/sql"
	"fmt"
	"testing"

	_ "github.com/jrhy/sandbox/sql/driver"
	_ "modernc.org/sqlite"

	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	require.NoError(t, err)
	fmt.Printf("%+v\n", db)
}

func TestQuery(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	// db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	rows, err := db.Query(`with foo(a,b) as (values(1,2),(3,4)) select * from foo`)
	require.NoError(t, err)
	cols, err := rows.Columns()
	require.NoError(t, err)
	require.Equal(t, "a", cols[0])
	require.Equal(t, "b", cols[1])
	require.Equal(t, 2, len(cols))
	require.True(t, rows.Next())
	var a, b int
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.Equal(t, 2, b)
	require.True(t, rows.Next())
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 3, a)
	require.Equal(t, 4, b)
	require.False(t, rows.Next())
}

func TestQueryEmpty(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	// db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	rows, err := db.Query(`with foo(a,b) as (values(1,2),(3,4)) select * from foo where a=5`)
	require.NoError(t, err)
	cols, err := rows.Columns()
	require.NoError(t, err)
	require.Equal(t, "a", cols[0])
	require.Equal(t, "b", cols[1])
	require.Equal(t, 2, len(cols))
	require.False(t, rows.Next())
}

func TestCreateAs(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	tableName := t.Name()
	// tableName := "foo"
	db, err := stdsql.Open("sandbox", ":memory:")
	// db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	// TODO: placeholders, maybe as fake table? see if it would play well aliasing an actual table with same name.
	_, err = db.Exec(fmt.Sprintf(`create table "%s" as with foo(a,b) as (values(1,2),(3,4)) select * from foo`, tableName))
	require.NoError(t, err)
	fmt.Printf("--- starting query \n")
	// TODO: did this really work when the table name matched the with name?
	rows, err := db.Query(fmt.Sprintf(`select * from "%s"`, tableName))
	require.NoError(t, err)
	cols, err := rows.Columns()
	require.NoError(t, err)
	require.Equal(t, 2, len(cols))
	require.Equal(t, "a", cols[0])
	require.Equal(t, "b", cols[1])
	require.True(t, rows.Next())
	var a, b int
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.Equal(t, 2, b)
	require.True(t, rows.Next())
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 3, a)
	require.Equal(t, 4, b)
	require.False(t, rows.Next())
}

func TestCreateInsert(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	// db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`create table "%s" (a,b)`, t.Name()))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`insert into "%s" values(1,2),(3,4)`, t.Name()))
	require.NoError(t, err)
	rows, err := db.Query(fmt.Sprintf(`select * from "%s"`, t.Name()))
	require.NoError(t, err)
	cols, err := rows.Columns()
	require.NoError(t, err)
	require.Equal(t, 2, len(cols))
	require.Equal(t, "a", cols[0])
	require.Equal(t, "b", cols[1])
	require.True(t, rows.Next())
	var a, b int
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.Equal(t, 2, b)
	require.True(t, rows.Next())
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 3, a)
	require.Equal(t, 4, b)
	require.False(t, rows.Next())
}

func TestJoin(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	// db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	rows, err := db.Query(`select * from (values(0),(1)) join (values(0),(1));`)
	require.NoError(t, err)
	cols, err := rows.Columns()
	require.NoError(t, err)
	require.Equal(t, 2, len(cols))
	require.True(t, rows.Next())
	var a, b int
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 0, a)
	require.Equal(t, 0, b)

	require.True(t, rows.Next())
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 0, a)
	require.Equal(t, 1, b)

	require.True(t, rows.Next())
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.Equal(t, 0, b)

	require.True(t, rows.Next())
	err = rows.Scan(&a, &b)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.Equal(t, 1, b)

	require.False(t, rows.Next())
}

func TestSelectNull(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	// db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	rows, err := db.Query(`select * from (values(null))`)
	require.NoError(t, err)
	cols, err := rows.Columns()
	require.NoError(t, err)
	require.Equal(t, 1, len(cols))
	require.True(t, rows.Next())
	var a interface{}
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, nil, a)

	require.False(t, rows.Next())
}

//TODO: values((select 1))

func TestDrop(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	//db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`create table "%s" (a,b)`, t.Name()))
	require.NoError(t, err)
	rows, err := db.Query(fmt.Sprintf(`select * from "%s"`, t.Name()))
	require.NoError(t, err)
	require.False(t, rows.Next())
	_, err = db.Exec(fmt.Sprintf(`drop table "%s"`, t.Name()))
	require.NoError(t, err)
	_, err = db.Query(fmt.Sprintf(`select c from "%s"`, t.Name()))
	require.Contains(t, err.Error(), t.Name())
	_, err = db.Exec(fmt.Sprintf(`create table "%s" (a,b)`, t.Name()))
	require.NoError(t, err)
	rows, err = db.Query(fmt.Sprintf(`select * from "%s"`, t.Name()))
	require.NoError(t, err)
	require.False(t, rows.Next())
}

func TODOTestCreateView(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	//db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`create table "%s" (a)`, t.Name()))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`insert into "%s" values(3),(1),(2)`, t.Name()))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`create view "%s_a" (a) as select * from "%s" order by a`, t.Name(), t.Name()))
	require.NoError(t, err)
	rows, err := db.Query(fmt.Sprintf(`select * from "%s"`, t.Name()))
	require.NoError(t, err)
	require.True(t, rows.Next())
	var a int
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 2, a)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 3, a)
	require.False(t, rows.Next())
}

func TODOTestSelectOrderBy(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	//db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`create table "%s" (a)`, t.Name()))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`insert into "%s" values(3),(1),(2)`, t.Name()))
	require.NoError(t, err)
	rows, err := db.Query(fmt.Sprintf(`select * from "%s" order by a`, t.Name()))
	require.NoError(t, err)
	require.True(t, rows.Next())
	var a int
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 2, a)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 3, a)
	require.False(t, rows.Next())
}

func TestCreateIndex(t *testing.T) {
	// TODO Parse() is not reentrant...
	// t.Parallel()
	db, err := stdsql.Open("sandbox", ":memory:")
	//db, err := stdsql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`create table "%s" (a)`, t.Name()))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`insert into "%s" values(3),(1),(2)`, t.Name()))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`create index "%s_a" on "%s"(a) where a!=2`, t.Name(), t.Name()))
	require.NoError(t, err)
	rows, err := db.Query(fmt.Sprintf(`select * from "%s_a"`, t.Name()))
	require.NoError(t, err)
	require.True(t, rows.Next())
	var a int
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 3, a)
	require.False(t, rows.Next())

	_, err = db.Exec(fmt.Sprintf(`insert into "%s" values(4)`, t.Name()))
	require.NoError(t, err)
	rows, err = db.Query(fmt.Sprintf(`select * from "%s_a"`, t.Name()))
	require.NoError(t, err)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 1, a)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 3, a)
	require.True(t, rows.Next())
	err = rows.Scan(&a)
	require.NoError(t, err)
	require.Equal(t, 4, a)
	require.False(t, rows.Next())
}
