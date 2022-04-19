package sql_test

import (
	"testing"

	"github.com/jrhy/sandbox/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: SELECT t1.*

func TestSchema_MixedQualified(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(
		// schema-qualified and unqualified column references
		`with foo(a,b) as (values(1,2),(3,4)) select bar.a,b from foo as bar`)
	require.NoError(t, err)
	require.NotNil(t, s.Select)
	require.Equal(t, []sql.SchemaColumn{{
		Source:       "foo",
		SourceColumn: "a",
		Name:         "a",
	}, {
		Source:       "foo",
		SourceColumn: "b",
		Name:         "b",
	}}, s.Select.Schema.Columns)
	require.Equal(t, &sql.Schema{
		Name: "foo",
		Columns: []sql.SchemaColumn{{
			Name: "a"}, {Name: "b"}},
	},
		s.Select.Schema.Sources["bar"])
	require.Equal(t, []sql.OutputExpression{
		{Expression: sql.SelectExpression{Column: &sql.Column{Term: "bar.a"}}},
		{Expression: sql.SelectExpression{Column: &sql.Column{Term: "b"}}},
	}, s.Select.Expressions)
}

func TestSelectWithoutFrom(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(`select current_timestamp`)
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestValues(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(`values('1',2,null,340282366920938463463374607431768211456),
		(3,4)`)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.NotNil(t, s.Values)
	assert.Equal(t, 2, len(s.Values.Rows))
	assert.NotNil(t, s.Values.Schema)
	assert.Equal(t, 4, len(s.Values.Schema.Columns))
	if t.Failed() {
		t.Logf("%v\n", s)
	}
}
func TestSelectValues(t *testing.T) {
	t.Parallel()
	s, err := sql.Parse(`select * from (values(1,2)),(values(3,4),(5,6));`)
	require.NoError(t, err)
	require.NotNil(t, s.Select)
	require.Equal(t, 1, len(s.Select.Values[0].Rows))
	require.Equal(t, 2, len(s.Select.Values[1].Rows))
	require.NotNil(t, s.Select.Schema)
	require.Equal(t, 4, len(s.Select.Schema.Columns))
	require.Equal(t, "column1", s.Select.Schema.Columns[0].Name)
	require.Equal(t, "column2", s.Select.Schema.Columns[1].Name)
	require.Equal(t, "column1", s.Select.Schema.Columns[2].Name)
	require.Equal(t, "column2", s.Select.Schema.Columns[3].Name)
}
func TODOTestSimulateOuterJoinLikeSqlite(t *testing.T) {
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
	t.Logf("%v\n", s)
}

func TestSchema_UnlabelledWith(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left as (values(0,1),(2,3)), right as (values(4,5),(6,7)) select * from left,right`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, 2, len(e.Select.With))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[0].Values.Schema))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[1].Values.Schema))
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

func TestSchema_LabelledWith(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left(la,lb) as (values(0,1),(2,3)), right(ra,rb) as (values(4,5),(6,7)) select * from left,right`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, 2, len(e.Select.With))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[0].Values.Schema))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[1].Values.Schema))
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

func TestSchema_RenamedWith(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left(la,lb) as (values(0,1),(2,3)), right(ra,rb) as (values(4,5),(6,7)) select la as oa, lb as ob, ra as oc, rb as od from left,right`)
	require.NoError(t, err)
	require.NotNil(t, e.Select.Schema)
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[0].Values.Schema))
	require.Equal(t, `{"Columns":[{"Name":"column1"},{"Name":"column2"}]}`, mustJSON(e.Select.With[1].Values.Schema))
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
