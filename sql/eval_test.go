package sql_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
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
	err = sql.Eval(e, nil, func(r *sql.Row) error {
		rows++
		fmt.Printf("got row %v\n", *r)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, rows)
}

func TestSelectUnrelated(t *testing.T) {
	t.Parallel()
	e, err := sql.Parse(
		`with left as (values(0),(1)), right as (values(0),(1)) select * from left,right`)
	require.NoError(t, err)
	assert.NotNil(t, e.Select.Schema)
	var rows []sql.Row
	err = sql.Eval(e, nil, func(r *sql.Row) error {
		rows = append(rows, *r)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 4, len(rows))
	assert.Equal(t, sql.Row([]sql.ColumnValue{colval.Int(0), colval.Int(0)}), rows[0])
	assert.Equal(t, sql.Row([]sql.ColumnValue{colval.Int(0), colval.Int(1)}), rows[1])
	assert.Equal(t, sql.Row([]sql.ColumnValue{colval.Int(1), colval.Int(0)}), rows[2])
	assert.Equal(t, sql.Row([]sql.ColumnValue{colval.Int(1), colval.Int(1)}), rows[3])
	if t.Failed() {
		t.Logf("%s\n", mustJSON(e.Select.Schema))
	}
}
