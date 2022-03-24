package sql_test

import (
	"testing"

	"github.com/jrhy/sandbox/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: SELECT t1.*

func TestSelect1(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(`select a as f,b from junk as mess`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Input)
	require.NotNil(t, s.Select)
	assert.Equal(t, []sql.OutputExpression{
		{Expression: sql.SelectExpression{Column: &sql.Column{Term: "a"}}, Alias: "f"},
		{Expression: sql.SelectExpression{Column: &sql.Column{Term: "b"}}},
	}, s.Select.Expressions)
	assert.Equal(t, []sql.Table{{Table: "junk", Alias: "mess"}}, s.Select.Tables)
}

func TestSelectWithoutFrom(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(`select current_timestamp`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Input)
}

func TestValues(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(`values('1',2,null,340282366920938463463374607431768211456),
		(3,4)`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Input)
	assert.NotNil(t, s.Values)
	assert.Equal(t, 2, len(s.Values.Rows))
	assert.NotNil(t, s.Values.Schema)
	assert.Equal(t, 4, len(s.Values.Schema.Columns))
	if t.Failed() {
		t.Logf("%s\n", s)
	}
}
func TestSelectValues(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(`select * from (values(1,2)),(values(3,4),(5,6));`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Input)
	assert.NotNil(t, s.Select)
	assert.Equal(t, 1, len(s.Select.Values[0].Rows))
	assert.Equal(t, 2, len(s.Select.Values[1].Rows))
	assert.NotNil(t, s.Select.Schema)
	assert.Equal(t, 6, len(s.Select.Schema.Columns))
	assert.Equal(t, "column1", s.Select.Schema.Columns[0].Name)
	assert.Equal(t, "column2", s.Select.Schema.Columns[1].Name)
	assert.Equal(t, "column1", s.Select.Schema.Columns[2].Name)
	assert.Equal(t, "column2", s.Select.Schema.Columns[3].Name)
	if t.Failed() {
		t.Logf("%s\n", s)
	}
}

func TestWith(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(
		`WITH t1 (c1) AS (
     VALUES (1)
	  , (NULL)
)
 SELECT COUNT(c1)
      , COUNT(*)
   FROM t1;
`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Input)
	t.Logf("%s\n", s)
}

func TestWith2(t *testing.T) {
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
	assert.Equal(t, "", s.Input)
	t.Logf("%s\n", s)
}

func TestSimulateOuterJoinLikeSqlite(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(
		`
WITH left (employee,department_id) AS (
     VALUES ('bob', 1)
	  , ('joe', 2)
	  , ('kat', 4)
), right (department_id, department_name) as (
     VALUES (1, 'forensics')
	  , (2, 'investigations')
	  , (3, 'complaints')
)
SELECT left.employee, right.department_name from left
left join right using(department_id) 
UNION ALL
SELECT left.employee, right.department_name from right
left join left using(department_id) where left.department_id is null
`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Input)
	t.Logf("%s\n", s)
}

func TestOuterJoin(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(
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
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "", s.Input)
	t.Logf("%s\n", s)
}
