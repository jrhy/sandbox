package sql_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jrhy/sandbox/parse"
	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestEvalSelectCurrentTimestamp(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(`select current_timestamp`)
	require.NoError(t, err)
	rows := 0
	err = sql.Eval(e, nil, func(r *types.Row) error {
		rows++
		fmt.Printf("got row %v\n", *r)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, rows)
}

func Test2WithValues(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left as (values(0),(1)), right as (values(0),(1)) select * from left,right`)
	require.NoError(t, err)
	require.Equal(t, 2, len(e.Select.With))
	require.Equal(t, 2, len(e.Select.With[0].Values.Rows))
	require.Equal(t, 1, len(e.Select.With[0].Values.Rows[0]))
	require.Equal(t, 2, len(e.Select.With[1].Values.Rows))
	require.Equal(t, 1, len(e.Select.With[1].Values.Rows[0]))
}

func getRows(rows *[]types.Row) func(*types.Row) error {
	return func(r *types.Row) error {
		fmt.Printf("row: %+v\n", *r)
		*rows = append(*rows, *r)
		return nil
	}
}

func TestSelectUnrelated(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left as (values(0),(1)), right as (values(0),(1)) select * from left,right`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	t.Logf("select schema: %s\n", mustJSON(e.Select.Schema))
	t.Logf("with[0] schema: %s\n", mustJSON(e.Select.With[0].Schema))
	t.Logf("with[0] values schema: %s\n", mustJSON(e.Select.With[0].Values.Schema))
	require.Equal(t, 4, len(rows))
	assert.Equal(t, types.Row([]colval.ColumnValue{colval.Int(0), colval.Int(0)}), rows[0])
	assert.Equal(t, types.Row([]colval.ColumnValue{colval.Int(0), colval.Int(1)}), rows[1])
	assert.Equal(t, types.Row([]colval.ColumnValue{colval.Int(1), colval.Int(0)}), rows[2])
	assert.Equal(t, types.Row([]colval.ColumnValue{colval.Int(1), colval.Int(1)}), rows[3])
}

func TestOutputExpression(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with foo(a,b) as (values(1,2),(3,4)) select b,a from foo`)
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, 2, len(rows))
	require.Equal(t, "2 1", rows[0].String())
	require.Equal(t, "4 3", rows[1].String())
}

func TestWith(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(
		`WITH t1 (c1) AS (
     VALUES (1)
	  , (NULL)
)
SELECT C1, * FROM t1;
`)
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(s, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[1,1]", mustJSON(rows[0]))
	require.Equal(t, "[{},{}]", mustJSON(rows[1]))
	fmt.Printf("%T %v\n", rows[1][0], rows[1][0])
}

func TODOTestCOUNT(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(
		`
		WITH t1 (c1) AS (
		     VALUES (1)
		          , (NULL)
		), t2 (c1) AS (
		     VALUES (1)
		          , (NULL)
                )
		 SELECT COUNT(t1.c1)
		      , COUNT(*)
		   FROM t1, t2;
		`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Parser.Remaining)
	t.Logf("%v\n", s)
}

func TestEval_UnlabelledWith(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left as (values(0,1),(2,3)), right as (values(4,5),(6,7)) select * from left,right`)
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[0,1,4,5],[0,1,6,7],[2,3,4,5],[2,3,6,7]]",
		mustJSON(rows))
}

func TestEval_LabelledWith(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left(la,lb) as (values(0,1),(2,3)), right(ra,rb) as (values(4,5),(6,7)) select * from left,right`)
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[0,1,4,5],[0,1,6,7],[2,3,4,5],[2,3,6,7]]",
		mustJSON(rows))
}

func TestEval_RenamedWith(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left(la,lb) as (values(0,1),(2,3)), right(ra,rb) as (values(4,5),(6,7)) select la as oa, lb as ob, ra as oc, rb as od from left,right`)
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[0,1,4,5],[0,1,6,7],[2,3,4,5],[2,3,6,7]]",
		mustJSON(rows))
}

func TestEval_RenamedWith_Subset(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left(la,lb) as (values(0,1),(2,3)), right(ra,rb) as (values(4,5),(6,7)) select la as oa, rb as od from left,right`)
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[0,5],[0,7],[2,5],[2,7]]",
		mustJSON(rows))
}

func TODOTestOuterJoin(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`WITH left (employee,department_id) AS (
     VALUES ('bob', 1)
	  , ('joe', 2)
	  , ('kat', 4)
), right (department_id, department_name) as (
     VALUES (1, 'forensics')
	  , (2, 'investigations')
	  , (3, 'complaints')
)
 SELECT employee, department_name from left
 outer join right using(department_id) 
`)
	// TODO: 'from left as l' and 'join right as r'
	// TODO: 'from left l' and 'join right r'
	t.Logf("schema: %s\n", mustJSON(e.Select.Schema))
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "", mustJSON(rows))
}

func TestCond_Equality(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`WITH foo(a) as (values(1),(2),(3),(4))
		SELECT a FROM foo WHERE a=3
`)
	require.NoError(t, err)
	var rows []types.Row
	err = sql.Eval(e, nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[3]]", mustJSON(rows))
}

func TODOTestOutput_InCondition(t *testing.T) {
	// `with foo(a,b) as (values(1,2)) select a+2 as calculated from foo where calculated=4`
}

func TestStringEscaping(t *testing.T) {
	t.Parallel()
	require.True(t, sql.StringValueRE.MatchString(`'hello'`))
	require.True(t, sql.StringValueRE.MatchString(`''`))
	require.True(t, sql.StringValueRE.MatchString(`''''`))
	require.True(t, sql.StringValueRE.MatchString(`"hello"`))
	require.True(t, sql.StringValueRE.MatchString(`""`))
	parseCV := func(s string) colval.ColumnValue {
		var cv colval.ColumnValue
		pf := sql.ColumnValueParser(&cv)
		p := &parse.Parser{Remaining: s}
		if p.Match(pf) && p.Remaining == "" {
			return cv
		} else {
			return colval.Null{}
		}
	}
	require.Equal(t, colval.Text(`hello`), parseCV(`'hello'`))
	require.Equal(t, colval.Text(`'`), parseCV(`''''`))
	require.Equal(t, colval.Text(`don't`), parseCV(`'don''t'`))
	require.Equal(t, colval.Text(`foo.bar`), parseCV(`"foo.bar"`))
	require.Equal(t, colval.Text(``), parseCV(`""`))
	checkSelect := func(stmt, expectedJSON string) {
		e, err := sql.Parse(stmt)
		require.NoError(t, err)
		var rows []types.Row
		err = sql.Eval(e, nil, getRows(&rows))
		require.NoError(t, err)
		require.Equal(t, expectedJSON, mustJSON(rows))
	}
	checkSelect(`WITH foo as (values(1))        SELECT * FROM foo`, `[[1]]`)
	checkSelect(`WITH foo as (values('texty'))  SELECT * FROM foo`, `[["texty"]]`)
	checkSelect(`WITH foo as (values("hello"))  SELECT * FROM foo`, `[["hello"]]`)
	checkSelect(`WITH foo as (values(''''))     SELECT * FROM foo`, `[["'"]]`)
	checkSelect(`WITH foo as (values('don''t')) SELECT * FROM foo`, `[["don't"]]`)
}

func TODOTestSQLiteEquivalence(t *testing.T) {
	t.Parallel()
	//check(`with 'don''t'(a) as (values(1)) select 'don''t'."a" from 'don''t'`, `1`)
}
