package sql_test

import (
	"testing"

	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: SELECT t1.*

func TODOTestParseSchema_MixedQualified(t *testing.T) {
	// don't know if this is relevant anymore. move to driver/expr
	t.Parallel()
	s, err := sql.Parse(nil,
		// schema-qualified and unqualified column references
		`with foo(a,b) as (values(1,2),(3,4)) select bar.a,b from foo as bar`)
	require.NoError(t, err)
	require.NotNil(t, s.Select)
	require.Equal(t, []types.SchemaColumn{{
		Source:       "foo",
		SourceColumn: "a",
		Name:         "a",
	}, {
		Source:       "foo",
		SourceColumn: "b",
		Name:         "b",
	}}, s.Select.Schema.Columns)
	require.Equal(t, &types.Schema{
		Name: "foo",
		Columns: []types.SchemaColumn{{
			Name: "a"}, {Name: "b"}},
	},
		s.Select.Schema.Sources["bar"])
	require.Equal(t, []types.OutputExpression{
		{Expression: types.SelectExpression{Column: &types.Column{Term: "bar.a"}}},
		{Expression: types.SelectExpression{Column: &types.Column{Term: "b"}}},
	}, s.Select.Expressions)
}

func TestParseSelectWithoutFrom(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(nil, `select current_timestamp`)
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestParseValues(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(nil, `values('1',2,null,340282366920938463463374607431768211456),
		(3,4,3e2,3.1e2),(1)`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.NotNil(t, s.Select.Values)
	assert.Equal(t, 3, len(s.Select.Values.Rows))
	assert.Equal(t, colval.Text("1"), s.Select.Values.Rows[0][0])
	assert.Equal(t, colval.Int(2), s.Select.Values.Rows[0][1])
	assert.Equal(t, colval.Null{}, s.Select.Values.Rows[0][2])
	assert.Equal(t, colval.Real(3.402823669209385e38), s.Select.Values.Rows[0][3])
	assert.Equal(t, colval.Int(3), s.Select.Values.Rows[1][0])
	assert.Equal(t, colval.Int(4), s.Select.Values.Rows[1][1])
	assert.Equal(t, colval.Real(300.0), s.Select.Values.Rows[1][2])
	assert.Equal(t, colval.Real(310.0), s.Select.Values.Rows[1][3])
	assert.NotNil(t, s.Select.Values.Schema)
	assert.Equal(t, 4, len(s.Select.Values.Schema.Columns))
	// TODO: this should fail since the third row has a different number of cols
	if t.Failed() {
		t.Logf("%v\n", s)
	}
}
func TestParseSelectValues(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(nil, `select * from (values(1,2)),(values(3,4),(5,6));`)
	require.NoError(t, err)
	require.NotNil(t, s.Select)
	require.Equal(t, 1, len(s.Select.FromItems[0].Subquery.Values.Rows))
	require.Equal(t, 2, len(s.Select.FromItems[1].Subquery.Values.Rows))
	require.NotNil(t, s.Select.Schema)
	require.Equal(t, 4, len(s.Select.Schema.Columns))
	require.Equal(t, "column1", s.Select.Schema.Columns[0].Name)
	require.Equal(t, "column2", s.Select.Schema.Columns[1].Name)
	require.Equal(t, "column1", s.Select.Schema.Columns[2].Name)
	require.Equal(t, "column2", s.Select.Schema.Columns[3].Name)
}
func TODOTestParseSimulateOuterJoinLikeSqlite(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(nil,
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
	t.Logf("%v\n", s)
}

func TODOTestParseSchema_UnlabelledWith(t *testing.T) {
	t.Parallel()
	// move to driver/expr
	e, err := sql.Parse(nil,
		`with left as (values(0,1),(2,3)), right as (values(4,5),(6,7)) select * from left,right`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, 2, len(e.Select.With))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[0].Select.Values.Schema))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[1].Select.Values.Schema))
	require.Equal(t, `[
  {
   "Source": "left",
   "SourceColumn": "column1",
   "Name": "column1"
  },
  {
   "Source": "left",
   "SourceColumn": "column2",
   "Name": "column2"
  },
  {
   "Source": "right",
   "SourceColumn": "column1",
   "Name": "column1"
  },
  {
   "Source": "right",
   "SourceColumn": "column2",
   "Name": "column2"
  }
 ]`, mustJSON(e.Select.Schema.Columns))
}

func TODOTestParseSchema_LabelledWith(t *testing.T) {
	// move to driver/expr
	t.Parallel()
	e, err := sql.Parse(nil,
		`with left(la,lb) as (values(0,1),(2,3)), right(ra,rb) as (values(4,5),(6,7)) select * from left,right`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, 2, len(e.Select.With))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[0].Select.Values.Schema))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[1].Select.Values.Schema))
	require.Equal(t, `{"Name":"left","Columns":[{"Name":"la"},{"Name":"lb"}]}`, mustJSON(e.Select.With[0].Schema))
	require.Equal(t, `{"Name":"right","Columns":[{"Name":"ra"},{"Name":"rb"}]}`, mustJSON(e.Select.With[1].Schema))
	require.Equal(t, `[
  {
   "Source": "left",
   "SourceColumn": "la",
   "Name": "la"
  },
  {
   "Source": "left",
   "SourceColumn": "lb",
   "Name": "lb"
  },
  {
   "Source": "right",
   "SourceColumn": "ra",
   "Name": "ra"
  },
  {
   "Source": "right",
   "SourceColumn": "rb",
   "Name": "rb"
  }
 ]`, mustJSON(e.Select.Schema.Columns))
}

func TODOTestParseSchema_RenamedWith(t *testing.T) {
	t.Parallel()
	// this isn't a useful parse test anymore, should be moved to driver or expr.
	e, err := sql.Parse(nil,
		`with left(la,lb) as (values(0,1),(2,3)), right(ra,rb) as (values(4,5),(6,7)) select la as oa, lb as ob, ra as oc, rb as od from left,right`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[0].Select.Values.Schema))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[1].Select.Values.Schema))
	require.Equal(t, `{"Name":"left","Columns":[{"Name":"la"},{"Name":"lb"}]}`, mustJSON(e.Select.With[0].Schema))
	require.Equal(t, `{"Name":"right","Columns":[{"Name":"ra"},{"Name":"rb"}]}`, mustJSON(e.Select.With[1].Schema))
	require.Equal(t, `[
  {
   "Source": "left",
   "SourceColumn": "la",
   "Name": "oa"
  },
  {
   "Source": "left",
   "SourceColumn": "lb",
   "Name": "ob"
  },
  {
   "Source": "right",
   "SourceColumn": "ra",
   "Name": "oc"
  },
  {
   "Source": "right",
   "SourceColumn": "rb",
   "Name": "od"
  }
 ]`, mustJSON(e.Select.Schema.Columns))
}

func TestParse_From3(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(nil,
		`with a as (values(0),(1)), b as (values(0),(1)), c as (values(0),(1)) 
		select * from a, b, c`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, `[
  {
   "Source": "a",
   "SourceColumn": "column1",
   "Name": "column1"
  },
  {
   "Source": "b",
   "SourceColumn": "column1",
   "Name": "column1"
  },
  {
   "Source": "c",
   "SourceColumn": "column1",
   "Name": "column1"
  }
 ]`, mustJSON(e.Select.Schema.Columns))
}

func TestParse_3WayJoin(t *testing.T) {
	t.Parallel()
	// this isn't a useful parse test anymore, should be moved to driver or expr.
	e, err := sql.Parse(nil,
		`with a as (values(0),(1)), b as (values(0),(1)), c as (values(0),(1)) 
		select * from a join b join c`)
	require.NoError(t, err)
	t.Logf("%s", mustJSON(e.Select))
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, `[
  {
   "Source": "a",
   "SourceColumn": "column1",
   "Name": "column1"
  },
  {
   "Source": "b",
   "SourceColumn": "column1",
   "Name": "column1"
  },
  {
   "Source": "c",
   "SourceColumn": "column1",
   "Name": "column1"
  }
 ]`, mustJSON(e.Select.Schema.Columns))
}

func TestParse_IndexesNotSupportedYet(t *testing.T) {
	t.Parallel()
	t.Run("ColumnConstraint", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`create table foo(a primary key)`)
		require.Error(t, err, "PRIMARY KEY is not supported yet")
	})
	t.Run("TableConstraint", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`create table foo(a) primary key (a )   )`)
		require.Error(t, err, "PRIMARY KEY is not supported yet")
	})
	t.Run("MultipleColumnsError", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`create table foo(a primary key, b primary key)`)
		require.Error(t, err, "PRIMARY KEY is not supported yet")
	})
	t.Run("MixedConstraints", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`create table foo(a primary key, primary key(a))`)
		require.Error(t, err, "PRIMARY KEY is not supported yet")
	})
}

func TestParse_UniqueNotSupportedYet(t *testing.T) {
	t.Parallel()
	t.Run("ColumnConstraint", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`create table foo(a unique)`)
		require.Error(t, err, "UNIQUE is not supported yet")
	})
	t.Run("TableConstraint", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`create table foo(a) unique (a )   )`)
		require.Error(t, err, "UNIQUE is not supported yet")
	})
	t.Run("MixedConstraints", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`create table foo(a unique, unique(a))`)
		require.Error(t, err, "UNIQUE is not supported yet")
	})
}

func TestParse_CreateTableWithTypes(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(nil,
		`create table foo(a text)`)
	require.NoError(t, err)
	require.Equal(t, e.Create.Schema.Columns[0].DefaultType, "text")
}

func TestParse_CreateTableWithColumnConstraints(t *testing.T) {
	t.Parallel()
	t.Run("NonNull", func(t *testing.T) {
		s, err := sql.Parse(nil,
			`create table foo(a text not null)`)
		require.NoError(t, err)
		require.True(t, s.Create.Schema.Columns[0].NotNull)
	})
	// t.Run("order1", func(t *testing.T) {
	// _, err := sql.Parse(nil,
	// `create table foo(a text unique not null)`)
	// require.NoError(t, err)
	// })
	// t.Run("order2", func(t *testing.T) {
	// _, err := sql.Parse(nil,
	// `create table foo(a text not null unique)`)
	// require.NoError(t, err)
	// })
}

func TODOTestParseGroupBy(t *testing.T) {
	t.Parallel()
	t.Run("Happy", func(t *testing.T) {
		_, err := sql.Parse(nil,
			`select * from (values(1),(2)) group by distinct 1`)
		require.NoError(t, err)
	})

}

func TestParseOrderBy(t *testing.T) {
	t.Parallel()
	t.Run("Happy", func(t *testing.T) {
		t.Parallel()
		s, err := sql.Parse(nil,
			`select * from (values(1),(2),(null)) order by 417 desc nulls first`)
		require.NoError(t, err)
		require.Equal(t, 1, len(s.Select.OrderBy))
		require.Nil(t, s.Select.OrderBy[0].Expression)
		require.Equal(t, 417, s.Select.OrderBy[0].OutputColumn)
		require.Equal(t, true, s.Select.OrderBy[0].NullsFirst)
		require.Equal(t, true, s.Select.OrderBy[0].Desc)
	})
}

func TestParseCreateIndex(t *testing.T) {
	t.Parallel()
	t.Run("Happy", func(t *testing.T) {
		t.Parallel()
		s, err := sql.Parse(nil,
			`create index foo_a on foo(a)`)
		require.NoError(t, err)
		require.NotNil(t, s.Create.Index)
		require.Equal(t, "foo_a", s.Create.Schema.Name)
		require.Equal(t, 1, len(s.Create.Schema.Columns))
		require.Nil(t, s.Create.Index.Expr)
	})
	t.Run("Partial", func(t *testing.T) {
		t.Parallel()
		s, err := sql.Parse(nil,
			`create index foo_a on foo(a) where a%2=0`)
		require.NoError(t, err)
		require.NotNil(t, s.Create.Index)
		require.Equal(t, "foo_a", s.Create.Schema.Name)
		require.Equal(t, 1, len(s.Create.Schema.Columns))
		require.NotNil(t, s.Create.Index.Expr)
	})
	/* TODO
	t.Run("Desc", func(t *testing.T) {
		t.Parallel()
		s, err := sql.Parse(nil,
			`create index foo_a on foo(a desc)`)
		require.NoError(t, err)
	})
	*/
}

func TestSelectLimit(t *testing.T) {
	s, err := sql.Parse(nil,
		`select * from (values (1),(2),(3),(4),(5)) limit -2--3 offset 1+-2;`)
	require.NoError(t, err)
	require.NotNil(t, s.Select)
	require.NotNil(t, s.Select.Limit)
	require.Equal(t, int64(1), *s.Select.Limit)
	require.NotNil(t, s.Select.Offset)
	require.Equal(t, int64(-1), *s.Select.Offset)
}
